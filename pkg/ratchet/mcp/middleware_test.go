package mcp_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet"
	ratchetmcp "github.com/hexxla/mcp-ratchet/pkg/ratchet/mcp"
)

func TestMiddlewareIsolatesSessionsAndReservesMatchingAlternative(t *testing.T) {
	ctx := t.Context()
	service, err := ratchet.New(ratchet.Policy{Rules: []ratchet.Rule{
		{Tool: "mutate", AfterAny: []string{"discover_a", "discover_b"}, Consume: true},
		{Tool: "finalize", AfterAny: []string{"mutate"}},
	}}, ratchet.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var mutations atomic.Int32
	srv := mcp.NewServer(&mcp.Implementation{Name: "ratchet-test", Version: "test"}, nil)
	srv.AddReceivingMiddleware(ratchetmcp.NewMiddleware(service))
	for _, name := range []string{"discover_a", "discover_b", "mutate", "finalize"} {
		handle := func(_ context.Context, _ *mcp.CallToolRequest, in struct {
			Fail bool `json:"fail"`
		}) (*mcp.CallToolResult, struct{}, error) {
			if name == "mutate" {
				mutations.Add(1)
			}
			return &mcp.CallToolResult{IsError: in.Fail}, struct{}{}, nil
		}
		mcp.AddTool(srv, &mcp.Tool{Name: string(name)}, handle)
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

func TestMiddlewareDoesNotRecordSDKFailures(t *testing.T) {
	for _, failure := range []string{"schema", "marshal", "handler", "tool_result", "input", "result_meta", "content_meta", "untyped_structured", "invalid_content"} {
		t.Run(failure, func(t *testing.T) {
			ctx := t.Context()
			service, err := ratchet.New(ratchet.Policy{Rules: []ratchet.Rule{
				{Tool: "discover", AfterAny: []string{"seed"}, Consume: true},
				{Tool: "mutate", AfterAny: []string{"discover"}},
			}}, ratchet.Options{})
			if err != nil {
				t.Fatal(err)
			}
			srv := mcp.NewServer(&mcp.Implementation{Name: "ratchet-test", Version: "test"}, nil)
			srv.AddReceivingMiddleware(ratchetmcp.NewMiddleware(service))
			for _, name := range []string{"seed", "mutate"} {
				mcp.AddTool(srv, &mcp.Tool{Name: name}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
					return nil, struct{}{}, nil
				})
			}
			type input struct {
				Count int `json:"count"`
			}
			type output struct {
				Value float64 `json:"value"`
			}
			tool := &mcp.Tool{Name: "discover"}
			if failure == "schema" {
				tool.OutputSchema = map[string]any{"type": "object", "required": []string{"missing"}}
			}
			var handlerCalls atomic.Int32
			mcp.AddTool(srv, tool, func(context.Context, *mcp.CallToolRequest, input) (*mcp.CallToolResult, any, error) {
				handlerCalls.Add(1)
				switch failure {
				case "marshal":
					return nil, output{Value: math.NaN()}, nil
				case "handler":
					return nil, output{}, errors.New("handler failed")
				case "tool_result":
					return &mcp.CallToolResult{IsError: true}, output{}, nil
				case "result_meta":
					return &mcp.CallToolResult{Meta: mcp.Meta{"invalid": make(chan int)}}, output{}, nil
				case "content_meta":
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Meta: mcp.Meta{"invalid": make(chan int)}}}}, output{}, nil
				case "untyped_structured":
					return &mcp.CallToolResult{StructuredContent: make(chan int)}, nil, nil
				case "invalid_content":
					return &mcp.CallToolResult{Content: []mcp.Content{nil}}, output{}, nil
				default:
					return nil, output{}, nil
				}
			})
			client := connectTestClient(t, srv)
			call := func(name string, arguments any) (*mcp.CallToolResult, error) {
				return client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
			}
			assertDenied := func(name string, arguments any) {
				t.Helper()
				result, err := call(name, arguments)
				if err == nil && result != nil && !result.IsError {
					t.Fatalf("%s succeeded after %s failure", name, failure)
				}
			}
			assertDenied("mutate", nil)
			if result, err := call("seed", nil); err != nil || result.IsError {
				t.Fatalf("seed: %+v, %v", result, err)
			}
			var arguments any = map[string]int{"count": 1}
			if failure == "input" {
				arguments = map[string]string{"count": "invalid"}
			}
			result, err := call("discover", arguments)
			if failure == "schema" || failure == "marshal" || failure == "result_meta" || failure == "content_meta" || failure == "untyped_structured" || failure == "invalid_content" {
				if err == nil {
					t.Fatalf("SDK output failure lost its protocol error: %+v", result)
				}
			} else if err != nil || result == nil || !result.IsError {
				t.Fatalf("SDK tool error was not preserved: %+v, %v", result, err)
			}
			assertDenied("mutate", nil)
			assertDenied("discover", map[string]int{"count": 1})
			wantCalls := int32(1)
			if failure == "input" {
				wantCalls = 0
			}
			if got := handlerCalls.Load(); got != wantCalls {
				t.Fatalf("handler called %d times, want %d; failed call must spend the seed", got, wantCalls)
			}
		})
	}
}

func TestMiddlewareRejectsInvalidRequests(t *testing.T) {
	for _, req := range []mcp.Request{
		nil,
		(*mcp.CallToolRequest)(nil),
		&mcp.ListToolsRequest{},
		&mcp.CallToolRequest{},
		&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
		&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "discover"}},
		&mcp.CallToolRequest{Session: &mcp.ServerSession{}, Params: &mcp.CallToolParamsRaw{Name: "discover"}},
	} {
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
			t.Fatal("invalid request reached the SDK handler")
			return nil, nil
		}
		result, err := ratchetmcp.NewMiddleware(nil)(next)(t.Context(), "tools/call", req)
		toolResult, ok := result.(*mcp.CallToolResult)
		if err != nil || !ok || !toolResult.IsError {
			t.Fatalf("invalid request did not fail closed: %+v, %v", result, err)
		}
	}
}

func TestMiddlewareRequiresExecutorAndPassesOtherMethods(t *testing.T) {
	var calls atomic.Int32
	srv := mcp.NewServer(&mcp.Implementation{Name: "ratchet-test", Version: "test"}, nil)
	srv.AddReceivingMiddleware(ratchetmcp.NewMiddleware(nil))
	mcp.AddTool(srv, &mcp.Tool{Name: "discover"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		calls.Add(1)
		return nil, struct{}{}, nil
	})
	client := connectTestClient(t, srv)
	if _, err := client.ListTools(t.Context(), nil); err != nil {
		t.Fatalf("middleware blocked tools/list: %v", err)
	}
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "discover"})
	if err != nil || result == nil || !result.IsError || calls.Load() != 0 {
		t.Fatalf("nil executor did not fail closed: %+v, %v, handler calls=%d", result, err, calls.Load())
	}
}

func connectTestClient(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(httpServer.Close)
	client, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// identifiedTransport supplies the stable identity normally owned by a host
// transport, while exercising the SDK's current protocol over in-memory I/O.
type identifiedTransport struct {
	mcp.Transport
	id string
}

type identifiedConnection struct {
	mcp.Connection
	id string
}

func (t identifiedTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	connection, err := t.Transport.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return identifiedConnection{Connection: connection, id: t.id}, nil
}

func (c identifiedConnection) SessionID() string { return c.id }

func TestMiddlewareInputRequiredDoesNotRecordCompletion(t *testing.T) {
	for _, loadShedding := range []bool{false, true} {
		t.Run(fmt.Sprintf("load_shedding=%t", loadShedding), func(t *testing.T) {
			engine, err := ratchet.New(ratchet.Policy{Rules: []ratchet.Rule{
				{Tool: "discover", AfterAny: []string{"seed"}, Consume: true},
				{Tool: "mutate", AfterAny: []string{"discover"}},
			}}, ratchet.Options{})
			if err != nil {
				t.Fatal(err)
			}
			srv := mcp.NewServer(&mcp.Implementation{Name: "continuation-test", Version: "test"}, nil)
			srv.AddReceivingMiddleware(ratchetmcp.NewMiddleware(engine))
			for _, name := range []string{"seed", "mutate"} {
				mcp.AddTool(srv, &mcp.Tool{Name: name}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
					return nil, struct{}{}, nil
				})
			}
			var calls atomic.Int32
			mcp.AddTool(srv, &mcp.Tool{Name: "discover"}, func(_ context.Context, request *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
				calls.Add(1)
				if request.Params.RequestState == "" {
					requests := mcp.InputRequestMap{}
					if !loadShedding {
						requests["confirm"] = &mcp.ElicitParams{Message: "Confirm the operation?"}
					}
					return &mcp.CallToolResult{InputRequests: requests, RequestState: "pending"}, nil, nil
				}
				return &mcp.CallToolResult{}, struct{}{}, nil
			})
			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			serverSession, err := srv.Connect(t.Context(), identifiedTransport{Transport: serverTransport, id: "isolated-client"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = serverSession.Close() })
			client, err := mcp.NewClient(&mcp.Implementation{Name: "continuation-client", Version: "test"}, &mcp.ClientOptions{
				MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true},
			}).Connect(t.Context(), clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			call := func(params *mcp.CallToolParams) *mcp.CallToolResult {
				t.Helper()
				result, err := client.CallTool(t.Context(), params)
				if err != nil || result == nil {
					t.Fatalf("%s call: %+v, %v", params.Name, result, err)
				}
				return result
			}
			if result := call(&mcp.CallToolParams{Name: "seed"}); result.IsError {
				t.Fatalf("seed denied: %+v", result)
			}
			pending := call(&mcp.CallToolParams{Name: "discover"})
			if !pending.NeedsInput() || pending.IsError || pending.RequestState != "pending" {
				t.Fatalf("continuation response changed: %+v", pending)
			}
			if pending.Meta[mcp.MetaKeyServerInfo] == nil {
				t.Fatal("snapshot dropped mandatory SDK server metadata")
			}
			if result := call(&mcp.CallToolParams{Name: "mutate"}); !result.IsError {
				t.Fatal("input-required result granted a downstream completion")
			}
			retry := &mcp.CallToolParams{Name: "discover", RequestState: pending.RequestState}
			if !loadShedding {
				retry.InputResponses = mcp.InputResponseMap{"confirm": &mcp.ElicitResult{Action: "accept"}}
			}
			if result := call(retry); !result.IsError {
				t.Fatal("continuation reused a spent one-time prerequisite")
			}
			if calls.Load() != 1 {
				t.Fatal("denied continuation reached the handler")
			}
			if result := call(&mcp.CallToolParams{Name: "seed"}); result.IsError {
				t.Fatal("fresh prerequisite denied")
			}
			if result := call(retry); result.IsError || result.NeedsInput() {
				t.Fatalf("completion after fresh prerequisite failed: %+v", result)
			}
			if result := call(&mcp.CallToolParams{Name: "mutate"}); result.IsError {
				t.Fatal("completed continuation did not grant its prerequisite")
			}
		})
	}
}
