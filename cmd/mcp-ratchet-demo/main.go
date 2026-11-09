package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
	ratchetPorts "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
	ratchetServices "github.com/hexxla/mcp-ratchet/pkg/ratchet/services"
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

	// Initialize ratchet service if config provided
	var workflow ratchetPorts.WorkflowExecutor
	if *ratchetConfig != "" {
		configLoader := adapters.NewYAMLConfigLoader()
		tokenStore := adapters.NewMemoryTokenStore()
		randomGen := adapters.NewCryptoRandomGenerator()
		clock := adapters.NewRealClock()

		configFile, err := os.Open(*ratchetConfig)
		if err != nil {
			return fmt.Errorf("failed to open ratchet config: %w", err)
		}
		ratchetSvc := ratchetServices.NewRatchetService(configLoader, tokenStore, randomGen, clock)
		rules, loadErr := ratchetSvc.LoadConfiguration(context.Background(), configFile)
		if err := errors.Join(loadErr, configFile.Close()); err != nil {
			return fmt.Errorf("failed to load ratchet configuration: %w", err)
		}
		workflow = ratchetSvc
		log.Info("Ratchet configuration loaded", "config", *ratchetConfig, "rules", len(rules))
	}

	// Create MCP server (primary adapter)
	srv := mcp.NewServer(&mcp.Implementation{Name: "mcp-ratchet-demo", Version: version}, nil)
	registerGreetingTool(srv, workflow, log)
	registerGetUserNameTool(srv, workflow, log)
	registerGetTimeTool(srv, workflow, log)
	registerGetDateTool(srv, workflow, log)

	// Create HTTP handler
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Logger: log})

	mux := http.NewServeMux()

	// MCP endpoint
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		mux.Handle(method+" "+listenPath, h)
	}

	httpSrv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errServe := make(chan error, 1)
	go func() {
		log.Info("MCP demo server listening", "addr", listenAddr, "path", listenPath, "version", version)
		errServe <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown failed: %w", err)
		}
		firstErr := <-errServe
		if firstErr != nil && !errors.Is(firstErr, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", firstErr)
		}
	case err := <-errServe:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
	}

	return nil
}
