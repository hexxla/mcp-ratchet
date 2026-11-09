// Package ratchet provides session-scoped workflow prerequisites for MCP tools.
// The services package loads policy and executes guarded calls. The mcp package
// adapts typed handlers to the executor. The embedding application owns
// authentication, access controls, and tool-specific result semantics.
package ratchet
