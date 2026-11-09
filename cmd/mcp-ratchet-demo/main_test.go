package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDemoHandlerSecurityAndSessionExpiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	srv := mcp.NewServer(&mcp.Implementation{Name: "demo-test", Version: "test"}, nil)
	httpServer := httptest.NewServer(demoHandler(srv, slog.New(slog.DiscardHandler), "/mcp", 20*time.Millisecond))
	defer httpServer.Close()
	defer func() {
		for session := range srv.Sessions() {
			_ = session.Close()
		}
	}()
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"test"}}}`
	request := func(method, origin, host string) (int, http.Header) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, httpServer.URL+"/mcp", strings.NewReader(initialize))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if origin != "" {
			req.Header.Set("Origin", origin)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
		}
		if host != "" {
			req.Host = host
		}
		resp, err := httpServer.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, resp.Header
	}
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		if status, _ := request(method, "https://untrusted.example", ""); status != http.StatusForbidden {
			t.Fatalf("cross-origin %s status=%d, want 403", method, status)
		}
	}
	if status, _ := request(http.MethodPost, "", "untrusted.example"); status != http.StatusForbidden {
		t.Fatalf("non-loopback Host status=%d, want 403", status)
	}
	if status, headers := request(http.MethodGet, "", ""); status != http.StatusMethodNotAllowed || headers.Get("Allow") != "DELETE, POST" {
		t.Fatalf("standalone SSE status=%d Allow=%q, want 405 and DELETE, POST", status, headers.Get("Allow"))
	}
	status, headers := request(http.MethodPost, "", "")
	if status != http.StatusOK || headers.Get("Mcp-Session-Id") == "" || !strings.HasPrefix(headers.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("initialize status=%d headers=%v; POST SSE must remain available", status, headers)
	}
	waitFor(t, ctx, func() bool {
		for session := range srv.Sessions() {
			if session.ID() == headers.Get("Mcp-Session-Id") {
				return false
			}
		}
		return true
	}, "idle MCP session to expire")
}

func TestServeDrainsActiveCallBeforeClosingSessions(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		name := "connected"
		if disconnect {
			name = "disconnected"
		}
		t.Run(name, func(t *testing.T) { testServeDrainsActiveCall(t, disconnect) })
	}
}

func testServeDrainsActiveCall(t *testing.T, disconnect bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	serveCtx, stop := context.WithCancel(ctx)
	defer stop()
	baseListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenerClosed := make(chan struct{})
	listener := &closeNotifyingListener{Listener: baseListener, notifyClosed: sync.OnceFunc(func() { close(listenerClosed) })}
	srv := mcp.NewServer(&mcp.Implementation{Name: "demo-test", Version: "test"}, nil)
	started, release, handlerCanceled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	releaseHandler := sync.OnceFunc(func() { close(release) })
	defer releaseHandler()
	mcp.AddTool(srv, &mcp.Tool{Name: "slow"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		close(started)
		select {
		case <-release:
			return nil, struct{}{}, nil
		case <-ctx.Done():
			close(handlerCanceled)
			return nil, struct{}{}, ctx.Err()
		}
	})
	serveResult := make(chan error, 1)
	go func() { serveResult <- serve(serveCtx, slog.New(slog.DiscardHandler), listener, srv, "/mcp") }()
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	defer httpTransport.CloseIdleConnections()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "demo-test", Version: "test"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: "http://" + listener.Addr().String() + "/mcp", HTTPClient: &http.Client{Transport: httpTransport}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		releaseHandler()
		stop()
		_ = client.Close()
	}()
	callResult := make(chan error, 1)
	callCtx, disconnectCall := context.WithCancel(ctx)
	defer disconnectCall()
	if disconnect {
		var sessionID string
		for session := range srv.Sessions() {
			sessionID = session.ID()
		}
		req, err := http.NewRequestWithContext(callCtx, http.MethodPost, "http://"+listener.Addr().String()+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":100,"method":"tools/call","params":{"name":"slow","arguments":{}}}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Mcp-Session-Id", sessionID)
		go func() {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_, err = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
			callResult <- err
		}()
	} else {
		go func() {
			result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "slow"})
			if err == nil && result.IsError {
				err = context.Canceled
			}
			callResult <- err
		}()
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("tool did not start")
	}
	if disconnect {
		disconnectCall()
		select {
		case err := <-callResult:
			if err == nil {
				t.Fatal("disconnected HTTP call unexpectedly succeeded")
			}
		case <-ctx.Done():
			t.Fatal("HTTP call did not disconnect")
		}
	}
	// The HTTP transport may finish a speculative dial without sending a
	// request. Shutdown legitimately waits over five seconds for StateNew
	// connections. Close only this fixture's idle connections so the test
	// observes active-call draining, without changing the production timeout.
	httpTransport.CloseIdleConnections()
	stop()
	select {
	case <-listenerClosed:
	case <-ctx.Done():
		t.Fatal("HTTP listener did not stop accepting calls")
	}
	select {
	case <-handlerCanceled:
		t.Fatal("shutdown canceled the active tool before draining it")
	default:
	}
	if disconnect {
		select {
		case err := <-serveResult:
			t.Fatalf("shutdown completed while a disconnected SDK handler was still active: %v", err)
		case <-handlerCanceled:
			t.Fatal("shutdown canceled a disconnected SDK handler instead of draining it")
		case <-time.After(20 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("shutdown did not begin")
		}
	}
	releaseHandler()
	if !disconnect {
		select {
		case err := <-callResult:
			if err != nil {
				t.Fatalf("active tool did not complete successfully: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("active tool did not drain")
		}
	}
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("graceful shutdown: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("server did not shut down")
	}
	for range srv.Sessions() {
		t.Fatal("shutdown retained an MCP session")
	}
}

func TestValidateDemoAddr(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8080", "[::1]:8080", "[::ffff:127.0.0.1]:8080"} {
		if err := validateDemoAddr(addr); err != nil {
			t.Fatalf("loopback %s rejected: %v", addr, err)
		}
	}
	for _, addr := range []string{":8080", "0.0.0.0:8080", "[::]:8080", "192.0.2.1:8080", "localhost:8080", "127.0.0.1"} {
		if err := validateDemoAddr(addr); err == nil {
			t.Fatalf("non-loopback or malformed address %s accepted", addr)
		}
	}
}

func waitFor(t *testing.T, ctx context.Context, ready func() bool, description string) {
	t.Helper()
	tick := time.Tick(time.Millisecond)
	for !ready() {
		select {
		case <-tick:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", description)
		}
	}
}

// closeNotifyingListener observes shutdown without creating a connection that
// net/http must itself classify and drain during the shutdown under test.
type closeNotifyingListener struct {
	net.Listener
	notifyClosed func()
}

func (l *closeNotifyingListener) Close() error {
	err := l.Listener.Close()
	l.notifyClosed()
	return err
}
