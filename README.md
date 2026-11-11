# MCP Ratchet

Ratchet is a Go library that enforces prerequisites between successful calls in
one session. A write can require a successful tag listing first. The core owns
process-local completion state; applications own authentication, resource access,
session identity, and the meaning of a successful result.

## Try the demo

With Go 1.27.4 or newer, run:

```sh
go run ./cmd/mcp-ratchet-demo -ratchet-config configs/ratchet.yaml
```

Connect an MCP client to `http://127.0.0.1:8080/mcp`. In one session,
`get_user_name` is denied until `greet` succeeds. Its completion is consumed
before the dependent call, so a failed call requires a fresh greeting.

The loopback demo accepts POST and DELETE, including POST SSE responses.
Standalone GET streams return 405. It rejects cross-origin mutations and
nonliteral endpoint paths, expires idle HTTP sessions after 30 minutes, and
drains active calls before closing MCP sessions.

## Configure a policy

```yaml
completion_ttl: 5m
rules:
  - tool: list_tags
    ttl: 10m
  - tool: put_cell
    after_any: [list_tags, find_tags]
    consume: true
    message: List or find tags before writing a cell.
  - tool: put_cell
    after_any: [cached_tags]
```

`after_any` groups alternatives. Ratchet tries rule blocks in file order and
names within each block in list order. `consume` spends the selected completion
before execution, including failed calls. Repeated blocks allow different
consumption choices, as the reusable `cached_tags` alternative above shows.
An omitted or empty `after_any` leaves the tool unrestricted.

`ttl` sets the lifetime of completions **issued by that rule's tool**. Omitted
values use `completion_ttl`, which defaults to five minutes. Positive explicit
TTLs for the same tool must agree. Tools without their own rule can still issue
completions when another rule names them as a prerequisite.

[`adapters.LoadPolicy`](pkg/ratchet/adapters/yaml_loader.go) accepts exactly one
YAML document. It rejects unknown fields, duplicate edges, cycles, conflicting
producer TTLs, and mixed guarded/unrestricted blocks for one tool. Explicit
YAML durations must be positive. The previous `prerequisite`, `expiry`,
`one_time_use`, `error_message`, and `observability` fields are rejected.

## Embed Ratchet

The public core is `github.com/hexxla/mcp-ratchet/pkg/ratchet`. It depends only
on the Go standard library. Construct a fixed policy directly or load YAML in
the separate adapter, then create one engine:

```go
engine, err := ratchet.New(ratchet.Policy{
    Rules: []ratchet.Rule{
        {Tool: "put_cell", AfterAny: []string{"list_tags"}, Consume: true},
    },
}, ratchet.Options{})
if err != nil {
    return err
}
srv.AddReceivingMiddleware(ratchetmcp.NewMiddleware(engine))
```

Install [`mcp.NewMiddleware`](pkg/ratchet/mcp/middleware.go) once and register
SDK tools normally. It records success after SDK validation and a complete JSON
snapshot of the result. Serialization or output-validation failures spend any
one-time prerequisite and create no downstream completion. SDK input-required
responses (including load shedding) also create no completion. Continuation RPCs
satisfy the policy again and need a fresh prerequisite when the prior RPC consumed
one; Ratchet keeps no pending continuation reservation.

Executed results become private snapshot wrappers implementing `mcp.Result`,
rather than retaining the dynamic Go type `*mcp.CallToolResult`. SDK interfaces
still expose metadata and `GetError`; clients receive ordinary typed SDK results.
Every original wire field is frozen, including input requests and request state,
while SDK server-info metadata decoration remains supported. Middleware that
decorates output must run inside Ratchet, before the snapshot.

Other transports call `engine.ExecuteToolCall(ctx, sessionID, tool, run)`.
`run` reports `(true, nil)` only after the full operation succeeds. False or an
error records no completion; handler errors are returned unchanged. False with
no error returns nil so a transport can retain its own failed result. Policy
input is cloned, including nested alternative lists. The policy cannot change
after construction.

Gate both dependent tools and prerequisites. One engine owns each session's
workflow and serializes its check, consumption, execution, and recording.
Queued callers honor cancellation; unrelated sessions can progress concurrently.
The engine tracks up to 1,024 sessions with active calls or live completions,
keeping the newest 64 completions per prerequisite tool. Denied and untracked
calls release empty state immediately. Expired completions and sessions idle
for 30 minutes are reclaimed during later activity, including admission at
capacity. State is lost on process restart.

## Observe events

Set `ratchet.Options{Observer: observe}` to receive typed `ratchet.Event`
values. They contain IDs, event type, session/tool names, timestamps, expiry,
consuming tool, and error text. There are no credentials or arbitrary metadata.
Callbacks run synchronously outside global bookkeeping locks; they must be
concurrency safe, return promptly, and avoid same-session engine reentry.
Redact session identifiers and error text before exposing diagnostics.

Optional history lives separately:

```go
history := adapters.NewMemoryEventStore(7)
defer history.Stop()
engine, err := ratchet.New(policy, ratchet.Options{Observer: history.Observe})
// Read history.Events(sessionID, filter) and history.Stats() independently.
```

History stores and returns detached event values. Positive retention days starts
a five-minute pruning loop; nonpositive retention keeps all events. The core
has no history-query facade. [Mosaic's integration](https://github.com/hexxla/mosaic/blob/main/docs/mosaic/RATCHET_INTEGRATION.md)
uses an authenticated observer with separate public correlation IDs.

The unreleased API removes the old domain/services/ports packages and
construction/storage abstractions. See [the changelog](CHANGELOG.md).

## Development

Run `make ci` for module checks, lint, race tests, builds, and vulnerability
scanning. The [architecture guide](docs/architecture/architecture.md) defines
package ownership and dependency boundaries.
