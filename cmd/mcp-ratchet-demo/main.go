package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/hexxla/mcp-ratchet/internal/adapter/primary/mcp"
	"github.com/hexxla/mcp-ratchet/internal/core/services"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
	ratchetDomain "github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	ratchetPorts "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
	ratchetSecondary "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
	ratchetServices "github.com/hexxla/mcp-ratchet/pkg/ratchet/services"
)

// WebSocket upgrader configuration
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for demo
	},
}

// eventBroadcaster manages WebSocket connections and broadcasts events
type eventBroadcaster struct {
	mu          sync.RWMutex
	connections map[ratchetDomain.SessionID]map[*websocket.Conn]struct{}
}

func newEventBroadcaster() *eventBroadcaster {
	return &eventBroadcaster{
		connections: make(map[ratchetDomain.SessionID]map[*websocket.Conn]struct{}),
	}
}

func (b *eventBroadcaster) subscribe(sessionID ratchetDomain.SessionID, conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.connections[sessionID] == nil {
		b.connections[sessionID] = make(map[*websocket.Conn]struct{})
	}
	b.connections[sessionID][conn] = struct{}{}
}

func (b *eventBroadcaster) unsubscribe(sessionID ratchetDomain.SessionID, conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if conns, ok := b.connections[sessionID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(b.connections, sessionID)
		}
	}
}

// Production implementations should add a broadcast method here and wire it
// into the EventStore or RatchetService to push events to WebSocket clients.
// See observability-improvements.md plan for details.

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
	var observabilityCfg ratchetDomain.ObservabilityConfig
	if *ratchetConfig != "" {
		configLoader := adapters.NewYAMLConfigLoader()
		tokenStore := adapters.NewMemoryTokenStore()
		sessionStore = adapters.NewMemorySessionStore()
		randomGen := adapters.NewCryptoRandomGenerator()
		clock := adapters.NewRealClock()

		// Load full configuration (rules + observability)
		configFile, err := os.Open(*ratchetConfig)
		if err != nil {
			return fmt.Errorf("failed to open ratchet config: %w", err)
		}
		defer func() {
			if closeErr := configFile.Close(); closeErr != nil {
				log.Warn("failed to close config file", "error", closeErr)
			}
		}()

		fullCfg, err := configLoader.LoadConfig(context.Background(), configFile)
		if err != nil {
			return fmt.Errorf("failed to load ratchet configuration: %w", err)
		}
		observabilityCfg = fullCfg.Observability

		// Create event store based on observability config
		eventStore, err := adapters.NewEventStore(fullCfg.Observability)
		if err != nil {
			return fmt.Errorf("failed to create event store: %w", err)
		}
		if eventStore != nil {
			log.Info("Ratchet observability enabled", "storage_type", fullCfg.Observability.StorageType)
		}

		ratchetSvc = ratchetServices.NewRatchetServiceWithObservability(configLoader, tokenStore, sessionStore, randomGen, clock, eventStore)

		// Register rules from loaded config
		for _, rule := range fullCfg.Rules {
			if err := ratchetSvc.RegisterRule(context.Background(), rule); err != nil {
				return fmt.Errorf("failed to register rule for tool %s: %w", rule.Tool, err)
			}
		}
		log.Info("Ratchet configuration loaded", "config", *ratchetConfig, "rules", len(fullCfg.Rules))
	}

	// Create MCP server (primary adapter)
	srv := mcp.NewServer("mcp-ratchet-demo", version, nil)
	mcp.RegisterGreetingTool(srv, greetingService, ratchetSvc, sessionStore, log)
	mcp.RegisterGetUserNameTool(srv, userService, ratchetSvc, sessionStore, log)
	mcp.RegisterGetTimeTool(srv, ratchetSvc, sessionStore, log)
	mcp.RegisterGetDateTool(srv, ratchetSvc, sessionStore, log)

	// Create HTTP handler
	h := mcp.StreamableHTTPHandler(srv, log)

	mux := http.NewServeMux()

	// Observability endpoints (web UI support)
	// GET /observability/stats - aggregate statistics
	// GET /observability/events?session_id=xxx - events for session
	if ratchetSvc != nil {
		mux.HandleFunc("GET /observability/stats", func(w http.ResponseWriter, r *http.Request) {
			stats, err := ratchetSvc.GetObservabilityStats(r.Context())
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to get stats: %v", err), http.StatusInternalServerError)
				return
			}
			if stats == nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "observability disabled"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(stats); err != nil {
				log.Warn("failed to encode stats", "error", err)
			}
		})

		mux.HandleFunc("GET /observability/events", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			sessionID := ratchetDomain.SessionID(q.Get("session_id"))

			// Build filter from query params
			filter := &ratchetSecondary.EventFilter{}

			// ?event_type=tool_call_failure,token_created (comma-separated)
			if raw := q.Get("event_type"); raw != "" {
				for t := range strings.SplitSeq(raw, ",") {
					filter.EventTypes = append(filter.EventTypes, ratchetDomain.EventType(strings.TrimSpace(t)))
				}
			}

			// ?tool_name=greet,get_user_name (comma-separated)
			if raw := q.Get("tool_name"); raw != "" {
				for t := range strings.SplitSeq(raw, ",") {
					filter.ToolNames = append(filter.ToolNames, ratchetDomain.ToolName(strings.TrimSpace(t)))
				}
			}

			// ?limit=50 (default 100)
			filter.Limit = 100
			if raw := q.Get("limit"); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > 0 {
					filter.Limit = n
				}
			}

			// ?offset=0 (pagination via EventFilter.Offset, applied post-query)
			offset := 0
			if raw := q.Get("offset"); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
					offset = n
				}
			}

			// Fetch with limit+offset to allow slicing
			filter.Limit += offset
			events, err := ratchetSvc.GetObservabilityEvents(r.Context(), sessionID, filter)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to get events: %v", err), http.StatusInternalServerError)
				return
			}

			// Apply offset
			if offset > 0 && offset < len(events) {
				events = events[offset:]
			} else if offset >= len(events) {
				events = []*ratchetDomain.Event{}
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(events); err != nil {
				log.Warn("failed to encode events", "error", err)
			}
		})
	}

	// WebSocket streaming endpoint (real-time events)
	// Connect with: wscat -c "ws://localhost:8080/observability/stream?session_id=demo-session"
	var broadcaster *eventBroadcaster
	if ratchetSvc != nil && observabilityCfg.WebSocketEnabled {
		broadcaster = newEventBroadcaster()

		mux.HandleFunc("GET /observability/stream", func(w http.ResponseWriter, r *http.Request) {
			sessionID := ratchetDomain.SessionID(r.URL.Query().Get("session_id"))
			if sessionID == "" {
				http.Error(w, "session_id required", http.StatusBadRequest)
				return
			}

			conn, err := wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Warn("websocket upgrade failed", "error", err)
				return
			}
			defer func() { _ = conn.Close() }()

			broadcaster.subscribe(sessionID, conn)
			defer broadcaster.unsubscribe(sessionID, conn)

			log.Info("websocket client connected", "session_id", sessionID, "remote_addr", r.RemoteAddr)

			// Send initial confirmation
			if err := conn.WriteJSON(map[string]string{
				"type":       "connected",
				"session_id": string(sessionID),
			}); err != nil {
				log.Warn("failed to send websocket confirmation", "error", err)
				return
			}

			// Keep connection alive and listen for client disconnect
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Warn("websocket error", "error", err)
					}
					break
				}
			}

			log.Info("websocket client disconnected", "session_id", sessionID)
		})

		log.Info("WebSocket streaming enabled", "endpoint", "/observability/stream")
		log.Info("Note: broadcaster not wired to event source in demo mode - production should wrap EventStore or service to broadcast")
		_ = broadcaster // TODO: Wire broadcaster to event source for production use
	}

	// MCP endpoint
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
		if ratchetSvc != nil {
			log.Info("Observability endpoints available", "stats", "/observability/stats", "events", "/observability/events")
		}
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
