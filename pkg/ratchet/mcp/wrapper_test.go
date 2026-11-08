package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	ratchetmcp "github.com/hexxla/mcp-ratchet/pkg/ratchet/mcp"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/services"
)

func TestWrapperIsolatesSessionsAndReservesMatchingAlternative(t *testing.T) {
	ctx := t.Context()
	sessions := adapters.NewMemorySessionStore()
	service := services.NewRatchetService(adapters.NewYAMLConfigLoader(), adapters.NewMemoryTokenStore(), sessions, adapters.NewCryptoRandomGenerator(), adapters.NewRealClock())
	for _, rule := range []domain.Rule{
		{Tool: "mutate", Prerequisite: "discover_a", OneTimeUse: true},
		{Tool: "mutate", Prerequisite: "discover_b", OneTimeUse: true},
		{Tool: "finalize", Prerequisite: "mutate"},
	} {
		if err := service.RegisterRule(ctx, rule); err != nil {
			t.Fatal(err)
		}
	}
	var mutations atomic.Int32
	srv := mcp.NewServer(&mcp.Implementation{Name: "ratchet-test", Version: "test"}, nil)
	for _, name := range []domain.ToolName{"discover_a", "discover_b", "mutate", "finalize"} {
		handle := func(_ context.Context, _ *mcp.CallToolRequest, in struct {
			Fail bool `json:"fail"`
		}) (*mcp.CallToolResult, struct{}, error) {
			if name == "mutate" {
				mutations.Add(1)
			}
			return &mcp.CallToolResult{IsError: in.Fail}, struct{}{}, nil
		}
		mcp.AddTool(srv, &mcp.Tool{Name: string(name)}, ratchetmcp.WrapWithRatchet(name, handle, service, sessions, nil))
	}
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(httpServer.Close)
	connect := func(name string) *mcp.ClientSession {
		t.Helper()
		session, err := mcp.NewClient(&mcp.Implementation{Name: name, Version: "test"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL, DisableStandaloneSSE: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	a, b := connect("a"), connect("b")
	call := func(client *mcp.ClientSession, tool string, fail bool) bool {
		result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: map[string]bool{"fail": fail}})
		return err == nil && result != nil && !result.IsError
	}
	if !call(a, "discover_b", false) || call(b, "mutate", false) {
		t.Fatal("prerequisite was unavailable to its own session or leaked to another")
	}
	var callers sync.WaitGroup
	var successes atomic.Int32
	for range 2 {
		callers.Go(func() {
			if call(a, "mutate", false) {
				successes.Add(1)
			}
		})
	}
	callers.Wait()
	if mutations.Load() != 1 || successes.Load() != 1 {
		t.Fatalf("one token allowed %d handler calls and %d successes", mutations.Load(), successes.Load())
	}
	if !call(b, "discover_b", false) || call(b, "mutate", true) || call(b, "finalize", false) || call(b, "mutate", false) {
		t.Fatal("failed handler granted a downstream token or kept its reserved prerequisite")
	}
	if mutations.Load() != 2 {
		t.Fatalf("failed handler path ran %d total mutations, want 2", mutations.Load())
	}
}
