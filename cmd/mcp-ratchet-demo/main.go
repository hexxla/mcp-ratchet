package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
	ratchetMCP "github.com/hexxla/mcp-ratchet/pkg/ratchet/mcp"
)

var version = "dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	addr := flag.String("addr", "", "override the demo listen address (default 127.0.0.1:8080)")
	path := flag.String("path", "", "override the MCP path (default /mcp)")
	ratchetConfig := flag.String("ratchet-config", "", "path to ratchet YAML config file (optional)")
	mcpConfig := flag.String("mcp-config", "configs/mcp-config.yaml", "path to MCP server config file")
	flag.Parse()

	// Load MCP server settings.
	mcpCfg, err := loadMCPConfig(*mcpConfig)
	if err != nil {
		log.Warn("failed to load MCP config, using defaults", "error", err)
		mcpCfg = &MCPConfig{}
	}

	listenAddr := cmp.Or(*addr, mcpCfg.Server.Addr, "127.0.0.1:8080")
	listenPath := cmp.Or(*path, mcpCfg.Server.MCPPath, "/mcp")
	if err := validateDemoAddr(listenAddr); err != nil {
		return err
	}
	if err := validateDemoPath(listenPath); err != nil {
		return err
	}

	// Initialize Ratchet only when a policy is provided.
	var workflow ratchetMCP.Executor
	if *ratchetConfig != "" {

		configFile, err := os.Open(*ratchetConfig)
		if err != nil {
			return fmt.Errorf("failed to open ratchet config: %w", err)
		}
		policy, loadErr := adapters.LoadPolicy(configFile)
		if err := errors.Join(loadErr, configFile.Close()); err != nil {
			return fmt.Errorf("failed to load ratchet configuration: %w", err)
		}
		workflow, err = ratchet.New(policy, ratchet.Options{})
		if err != nil {
			return fmt.Errorf("create Ratchet engine: %w", err)
		}
		log.Info("Ratchet policy loaded", "config", *ratchetConfig, "rules", len(policy.Rules))
	}

	// Create MCP server (primary adapter)
	srv := mcp.NewServer(&mcp.Implementation{Name: "mcp-ratchet-demo", Version: version}, nil)
	if workflow != nil {
		srv.AddReceivingMiddleware(ratchetMCP.NewMiddleware(workflow))
	}
	registerGreetingTool(srv, log)
	registerGetUserNameTool(srv, log)
	registerGetTimeTool(srv, log)
	registerGetDateTool(srv, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen for MCP requests: %w", err)
	}
	return serve(ctx, log, listener, srv, listenPath)
}

const demoSessionTimeout = 30 * time.Minute

func demoHandler(srv *mcp.Server, log *slog.Logger, path string, sessionTimeout time.Duration) http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		Logger: log, SessionTimeout: sessionTimeout,
	})
	mux := http.NewServeMux()
	// The demo sends SSE responses to POST calls, but has no server pushes that
	// require an indefinitely open standalone GET stream.
	mux.Handle("POST "+path, h)
	mux.Handle("DELETE "+path, h)
	return http.NewCrossOriginProtection().Handler(mux)
}

func serve(ctx context.Context, log *slog.Logger, listener net.Listener, srv *mcp.Server, path string) error {
	calls := &activeToolCalls{drained: make(chan struct{})}
	srv.AddReceivingMiddleware(calls.middleware)
	httpSrv := &http.Server{
		Handler:           demoHandler(srv, log, path, demoSessionTimeout),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errServe := make(chan error, 1)
	go func() {
		log.Info("MCP demo server listening", "addr", listener.Addr(), "path", path, "version", version)
		errServe <- httpSrv.Serve(listener)
	}()

	var serveErr error
	serveReturned := false
	select {
	case <-ctx.Done():
	case serveErr = <-errServe:
		serveReturned = true
	}
	drained := calls.stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Stop accepting requests, then drain both HTTP responses and SDK calls.
	// An SDK call can outlive its POST connection if the client disconnects.
	shutdownErr := httpSrv.Shutdown(shutdownCtx)
	select {
	case <-drained:
	case <-shutdownCtx.Done():
		shutdownErr = errors.Join(shutdownErr, shutdownCtx.Err())
	}
	if shutdownErr != nil {
		shutdownErr = fmt.Errorf("server shutdown failed: %w", errors.Join(shutdownErr, httpSrv.Close()))
	}
	if !serveReturned {
		serveErr = <-errServe
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	var sessionErr error
	for session := range srv.Sessions() {
		if err := session.Close(); err != nil {
			sessionErr = errors.Join(sessionErr, fmt.Errorf("close MCP session: %w", err))
		}
	}
	return errors.Join(serveErr, shutdownErr, sessionErr)
}

// activeToolCalls closes admission during shutdown and tracks handlers even when
// their HTTP connection has already gone away.
type activeToolCalls struct {
	mu       sync.Mutex
	active   int
	stopping bool
	drained  chan struct{}
}

func (c *activeToolCalls) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method != "tools/call" {
			return next(ctx, method, req)
		}
		c.mu.Lock()
		if c.stopping {
			c.mu.Unlock()
			result := &mcp.CallToolResult{}
			result.SetError(errors.New("MCP demo server is shutting down"))
			return result, nil
		}
		c.active++
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.active--
			if c.stopping && c.active == 0 {
				close(c.drained)
			}
		}()
		return next(ctx, method, req)
	}
}

func (c *activeToolCalls) stop() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopping {
		c.stopping = true
		if c.active == 0 {
			close(c.drained)
		}
	}
	return c.drained
}
