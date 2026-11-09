package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet"
)

const snapshotValue = `{"large":9007199254740993,"null":null,"empty":[],"text":"<>&"}`

type onceMarshaler struct{ calls atomic.Int32 }

func (m *onceMarshaler) MarshalJSON() ([]byte, error) {
	if m.calls.Add(1) != 1 {
		return nil, errors.New("user marshaler evaluated more than once")
	}
	return []byte(snapshotValue), nil
}

func snapshotFixture(value func() any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Meta: mcp.Meta{"large": int64(9007199254740993), "value": value()},
		Content: []mcp.Content{
			&mcp.TextContent{Text: "", Meta: mcp.Meta{"value": value()}},
			&mcp.ImageContent{MIMEType: "image/png"},
			&mcp.AudioContent{MIMEType: "audio/wav", Data: []byte{}},
			&mcp.ResourceLink{URI: "file:///example", Name: "example", Size: new(int64(9007199254740993)), Meta: mcp.Meta{"large": int64(9007199254740993)}},
			&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "file:///example", Text: "<>&", Meta: mcp.Meta{"value": value()}}},
		},
		StructuredContent: value(),
	}
}

func encodeSnapshot(t *testing.T, result mcp.Result) []byte {
	t.Helper()
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFreezeToolResultPreservesWireJSON(t *testing.T) {
	var marshalers []*onceMarshaler
	source := snapshotFixture(func() any {
		value := new(onceMarshaler)
		marshalers = append(marshalers, value)
		return value
	})
	frozen, err := freezeToolResult(source)
	if err != nil {
		t.Fatal(err)
	}
	// Mutating the handler's original objects cannot alter the frozen response.
	source.Meta["large"] = "changed"
	source.Content[0].(*mcp.TextContent).Text = "changed"
	source.Content[4].(*mcp.EmbeddedResource).Resource.Meta["value"] = "changed"
	want := encodeSnapshot(t, snapshotFixture(func() any { return json.RawMessage(snapshotValue) }))
	for range 2 {
		if got := encodeSnapshot(t, frozen); !sameSnapshotJSON(t, got, want) {
			t.Fatalf("wire JSON changed:\n got %s\nwant %s", got, want)
		}
	}
	for _, value := range marshalers {
		if got := value.calls.Load(); got != 1 {
			t.Fatalf("user marshaler evaluated %d times, want once", got)
		}
	}
	var clientResult mcp.CallToolResult
	if err := clientResult.UnmarshalJSON(want); err != nil {
		t.Fatal(err)
	}
	if len(clientResult.Content) != 5 || clientResult.Content[0].(*mcp.TextContent).Text != "" || clientResult.Content[4].(*mcp.EmbeddedResource).Resource.Text != "<>&" {
		t.Fatalf("SDK content roundtrip changed: %+v", clientResult)
	}
}

func TestFreezeToolResultPreservesNullAndEmpty(t *testing.T) {
	for _, source := range []*mcp.CallToolResult{
		{},
		{Content: []mcp.Content{}},
		{Meta: mcp.Meta{"null": nil, "empty": []string{}}, StructuredContent: map[string]string(nil)},
		{StructuredContent: map[string]string{}},
		{InputRequests: mcp.InputRequestMap{}},
		{InputRequests: mcp.InputRequestMap{"confirm": &mcp.ElicitParams{Message: "Confirm?"}}, RequestState: "pending"},
	} {
		want := encodeSnapshot(t, source)
		frozen, err := freezeToolResult(source)
		if err != nil {
			t.Fatal(err)
		}
		if got := encodeSnapshot(t, frozen); !sameSnapshotJSON(t, got, want) {
			t.Fatalf("null/empty wire JSON changed: got %s, want %s", got, want)
		}
	}
	source := &mcp.CallToolResult{}
	cause := errors.New("tool failed")
	source.SetError(cause)
	frozen, err := freezeToolResult(source)
	if err != nil || !frozen.IsError || !errors.Is(frozen.GetError(), cause) {
		t.Fatalf("SDK error metadata lost: %+v, %v", frozen, err)
	}
}

func TestMiddlewareSendsFrozenResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	service, err := ratchet.New(ratchet.Policy{Rules: []ratchet.Rule{{Tool: "mutate", AfterAny: []string{"discover"}}}}, ratchet.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var marshalers []*onceMarshaler
	source := snapshotFixture(func() any {
		value := new(onceMarshaler)
		marshalers = append(marshalers, value)
		return value
	})
	srv := mcp.NewServer(&mcp.Implementation{Name: "snapshot-test", Version: "test"}, nil)
	srv.AddReceivingMiddleware(NewMiddleware(service))
	mcp.AddTool(srv, &mcp.Tool{Name: "discover"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return source, nil, nil
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "mutate"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	defer httpServer.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "snapshot-test", Version: "test"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "discover"})
	if err != nil || result == nil || result.IsError || len(result.Content) != 5 {
		t.Fatalf("frozen result did not reach the SDK client: %+v, %v", result, err)
	}
	if result.Content[0].(*mcp.TextContent).Meta["value"] == nil || result.Content[4].(*mcp.EmbeddedResource).Resource.Meta["value"] == nil {
		t.Fatalf("content metadata did not reach the SDK client: %+v", result)
	}
	for _, value := range marshalers {
		if got := value.calls.Load(); got != 1 {
			t.Fatalf("user marshaler evaluated %d times across middleware and wire encoding", got)
		}
	}
	if result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "mutate"}); err != nil || result.IsError {
		t.Fatalf("successful frozen result did not grant its prerequisite: %+v, %v", result, err)
	}
}

func TestFreezeToolResultDetachesInputRequests(t *testing.T) {
	value := new(onceMarshaler)
	request := &mcp.ElicitParams{Message: "confirm", Meta: mcp.Meta{"value": value}, RequestedSchema: map[string]any{"large": int64(9007199254740993), "null": nil, "empty": []any{}}}
	source := &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{"confirm": request}, RequestState: "pending"}
	frozen, err := freezeToolResult(source)
	if err != nil {
		t.Fatal(err)
	}
	// Every original object can change after the snapshot without changing wire data.
	request.Message = "changed"
	request.RequestedSchema.(map[string]any)["large"] = "changed"
	delete(source.InputRequests, "confirm")
	source.RequestState = "changed"
	want := encodeSnapshot(t, &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{"confirm": &mcp.ElicitParams{Message: "confirm", Meta: mcp.Meta{"value": json.RawMessage(snapshotValue)}, RequestedSchema: map[string]any{"large": int64(9007199254740993), "null": nil, "empty": []any{}}}}, RequestState: "pending"})
	for range 2 {
		if got := encodeSnapshot(t, frozen); !sameSnapshotJSON(t, got, want) {
			t.Fatalf("input request snapshot changed:\n got %s\nwant %s", got, want)
		}
	}
	if value.calls.Load() != 1 {
		t.Fatalf("input request custom marshaler called %d times", value.calls.Load())
	}
}

func sameSnapshotJSON(t *testing.T, first, second []byte) bool {
	t.Helper()
	decode := func(data []byte) any {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	return reflect.DeepEqual(decode(first), decode(second))
}

func TestFreezeToolResultInputRequestMarshalerOnce(t *testing.T) {
	value := new(onceMarshaler)
	source := &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{"confirm": &mcp.ElicitParams{Message: "Confirm?", Meta: mcp.Meta{"value": value}}}}
	frozen, err := freezeToolResult(source)
	if err != nil {
		t.Fatal(err)
	}
	want := encodeSnapshot(t, &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{"confirm": &mcp.ElicitParams{Message: "Confirm?", Meta: mcp.Meta{"value": json.RawMessage(snapshotValue)}}}})
	for range 2 {
		if got := encodeSnapshot(t, frozen); !sameSnapshotJSON(t, got, want) {
			t.Fatalf("input request values changed: %s", got)
		}
	}
	if value.calls.Load() != 1 {
		t.Fatalf("input request marshaler called %d times", value.calls.Load())
	}
}

func TestFreezeToolResultRetainsSDKMetadataDecoration(t *testing.T) {
	value := new(onceMarshaler)
	source := &mcp.CallToolResult{Meta: mcp.Meta{"large": int64(9007199254740993), "value": value}}
	frozen, err := freezeToolResult(source)
	if err != nil {
		t.Fatal(err)
	}
	meta := frozen.GetMeta()
	meta[mcp.MetaKeyServerInfo] = &mcp.Implementation{Name: "server", Version: "test"}
	frozen.SetMeta(meta)
	want := encodeSnapshot(t, &mcp.CallToolResult{Meta: mcp.Meta{"large": int64(9007199254740993), "value": json.RawMessage(snapshotValue), mcp.MetaKeyServerInfo: &mcp.Implementation{Name: "server", Version: "test"}}})
	if got := encodeSnapshot(t, frozen); !sameSnapshotJSON(t, got, want) {
		t.Fatalf("SDK decoration changed original metadata: %s", got)
	}
	if value.calls.Load() != 1 {
		t.Fatalf("metadata marshaler called %d times", value.calls.Load())
	}
}
