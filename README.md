# MCP Ratchet

MCP Ratchet is a Go library that enforces tool prerequisites within one MCP
session. For example, a write can require a successful tag listing first.
Ratchet checks tool completion and ordering. The embedding application owns
authentication, resource authorization, and the meaning of a tool's result.

## Try the demo

With Go 1.27.3 or newer, run the loopback demo from the repository root.

```sh
go run ./cmd/mcp-ratchet-demo -ratchet-config configs/ratchet.yaml
```

Connect an MCP client to `http://127.0.0.1:8080/mcp`. In one session,
`get_user_name` is denied until `greet` succeeds. Its one-time prerequisite
is spent before the handler runs, so a failed call needs a fresh prerequisite.

## Configure a policy

```yaml
rules:
  - tool: list_tags
    expiry: 10m
  - tool: put_cell
    prerequisite: list_tags
    one_time_use: true
    error_message: List tags before writing a cell.
```

`expiry` controls the lifetime of completions **issued by** the rule's
`tool`. It defaults to five minutes and explicit values for the same tool
must agree. Rules for one dependent tool are alternatives, evaluated in file
order. Unguarded tools can be prerequisites. Unknown fields, duplicate
edges, cycles, and mixed guarded and unguarded rules for the same tool are
rejected.

## Embed Ratchet

Load rules with `services.NewRatchetService` and `LoadConfiguration`.
Use [`ExecuteToolCall`](pkg/ratchet/ports/primary/ratchet_service.go) from
a middleware that gets the session ID from the MCP transport, or wrap typed
handlers with [`WrapWithRatchet`](pkg/ratchet/mcp/wrapper.go).

```go
handler = ratchetmcp.WrapWithRatchet("put_cell", handler, service)
```

Gate both the protected tools and their prerequisites. The executor serializes
calls within a session, waits with request cancellation, and tracks successful
completions only for tools used as prerequisites. State lives in process.
The default executor admits up to 1,024 sessions. Each session keeps up to
64 completions per prerequisite tool, retaining the newest at capacity.
Idle sessions expire after 30 minutes and are reclaimed on subsequent
activity. Expired completions cannot authorize a call.

For observability, applications can pass a custom event store to
`NewRatchetServiceWithObservability`. Event stores receive internal session
IDs and tokens. Redact or replace those values before exposing events.
[Mosaic's integration](https://github.com/hexxla/mosaic/blob/main/docs/mosaic/RATCHET_INTEGRATION.md)
has an authenticated live observer that emits separate correlation IDs.

## Development

Run `make ci` to check the module, lint, race tests, command build, and
vulnerability scan. See the [architecture guide](docs/architecture/architecture.md)
for package boundaries.
