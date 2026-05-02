package main

import (
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

	"github.com/hexxla/mcp-ratchet/internal/adapter/primary/mcp"
	"github.com/hexxla/mcp-ratchet/internal/core/services"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
	ratchetPorts "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
	ratchetSecondary "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
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
	addr := flag.String("addr", ":8080", "address to listen on")
	path := flag.String("path", "/mcp", "MCP server path")
	ratchetConfig := flag.String("ratchet-config", "", "path to ratchet YAML config file (optional)")
	flag.Parse()

	// Create service layer (implements primary ports)
	greetingService := services.NewGreetingService()
	userService := services.NewUserService()

	// Initialize ratchet service if config provided
	var ratchetSvc ratchetPorts.RatchetService
	var sessionStore ratchetSecondary.SessionStore
	if *ratchetConfig != "" {
		configLoader := adapters.NewYAMLConfigLoader()
		tokenStore := adapters.NewMemoryTokenStore()
		sessionStore = adapters.NewMemorySessionStore()
		randomGen := adapters.NewCryptoRandomGenerator()
		clock := adapters.NewRealClock()
		ratchetSvc = ratchetServices.NewRatchetService(configLoader, tokenStore, sessionStore, randomGen, clock)

		// Load configuration
		configFile, err := os.Open(*ratchetConfig)
		if err != nil {
			return fmt.Errorf("failed to open ratchet config: %w", err)
		}
		defer func() {
			if closeErr := configFile.Close(); closeErr != nil {
				log.Warn("failed to close config file", "error", closeErr)
			}
		}()

		_, err = ratchetSvc.LoadConfiguration(context.Background(), configFile)
		if err != nil {
			return fmt.Errorf("failed to load ratchet configuration: %w", err)
		}
		log.Info("Ratchet configuration loaded", "config", *ratchetConfig)
	}

	// Create MCP server (primary adapter)
	srv := mcp.NewServer("mcp-ratchet-demo", version, nil)
	mcp.RegisterGreetingTool(srv, greetingService, ratchetSvc, sessionStore, log)
	mcp.RegisterGetUserNameTool(srv, userService, ratchetSvc, sessionStore, log)
	mcp.RegisterGetTimeTool(srv, ratchetSvc, sessionStore, log)

	// Create HTTP handler
	h := mcp.StreamableHTTPHandler(srv, log)

	mux := http.NewServeMux()
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		mux.Handle(method+" "+*path, h)
	}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errServe := make(chan error, 1)
	go func() {
		log.Info("MCP demo server listening", "addr", *addr, "path", *path, "version", version)
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
