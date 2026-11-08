# MCP Ratchet architecture

Ratchet is a reusable Go library for prerequisites between tool calls. The application core owns session state, prerequisite selection and one-time reservation.

| Path | Ownership |
| --- | --- |
| `pkg/ratchet/domain` | Rules, session state and event values |
| `pkg/ratchet/ports/primary` | Public service and workflow execution contracts |
| `pkg/ratchet/ports/secondary` | Session and token stores, clock, entropy, config and event contracts |
| `pkg/ratchet/services` | Session-scoped workflow execution and application state transitions |
| `pkg/ratchet/adapters` | Memory, YAML, clock, entropy and event implementations |
| `pkg/ratchet/mcp` | SDK adapter that obtains the transport session ID and maps tool results |
| `cmd/mcp-ratchet-demo` | Loopback demonstration, composition and redacted diagnostics |

The core depends on the domain and ports. The MCP adapter depends on the primary port. A transport with an authenticated or otherwise isolated session identity supplies that ID to the workflow. Each session's reservation, handler execution and recording are serialized. An eligible one-time alternative reserves the exact token before the handler. A failed handler spends the reservation and issues no downstream token. Session store callers receive detached snapshots.

Use `primary.WorkflowExecutor` for new integrations. Lower-level methods remain available for callers that manage their own sequencing. The in-memory implementation is process-local. The demo is for local observation and uses loopback IP addresses.

Run `make ci` for boundary checks and the public adapter's two-client integration test.
