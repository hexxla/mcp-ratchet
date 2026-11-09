// Package ratchet enforces session-scoped prerequisites between successful calls.
// New validates an immutable policy; Engine owns bounded process-local completion
// state and serializes each session's check, consumption, call and recording.
// The core has no MCP SDK or YAML dependency. The adapters package loads YAML and
// optionally records event history; mcp provides finalized-result middleware.
// Embedding applications own session identity, authentication, resource access,
// and the meaning of a successful tool result.
package ratchet
