package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// freezeToolResult snapshots every wire field before recording success. JSON v1
// matches the SDK's wire encoder; original user marshalers run only here.
// The detached SDK view supplies sealed Result methods and inspection, while
// raw fields preserve numeric values and null/empty shapes on later encoding.
func freezeToolResult(result *mcp.CallToolResult) (*frozenToolResult, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return nil, fmt.Errorf("marshaling tool result: %w", err)
	}
	var view mcp.CallToolResult
	if err := view.UnmarshalJSON(buf.Bytes()); err != nil {
		return nil, fmt.Errorf("decoding tool result: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &fields); err != nil {
		return nil, fmt.Errorf("decoding tool result fields: %w", err)
	}
	var meta map[string]json.RawMessage
	if data := fields["_meta"]; data != nil {
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("decoding tool result metadata: %w", err)
		}
	}
	view.Meta = nil
	if meta != nil {
		view.Meta = make(mcp.Meta, len(meta))
		for key, value := range meta {
			view.Meta[key] = value
		}
	}
	return &frozenToolResult{CallToolResult: &view, fields: fields, cause: result.GetError()}, nil
}

// The SDK adds serverInfo metadata after receiving middleware returns. Preserve
// that decoration while retaining raw JSON for all original user-owned values.
// Output-decorating application middleware must run inside Ratchet.
type frozenToolResult struct {
	*mcp.CallToolResult
	fields map[string]json.RawMessage
	cause  error
}

func (r *frozenToolResult) GetError() error { return r.cause }

func (r *frozenToolResult) MarshalJSON() ([]byte, error) {
	fields := maps.Clone(r.fields)
	delete(fields, "_meta")
	if len(r.Meta) != 0 {
		data, err := json.Marshal(r.Meta)
		if err != nil {
			return nil, err
		}
		fields["_meta"] = data
	}
	return json.Marshal(fields)
}
