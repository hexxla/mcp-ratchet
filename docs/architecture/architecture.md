# MCP Ratchet architecture

Ratchet keeps prerequisite policy and process-local session state together in
one Go package. Parsing, history, and the MCP SDK have separate owners.

| Path | Responsibility and dependencies |
| --- | --- |
| `pkg/ratchet` | Policy, validation, immutable events and Engine; Go standard library only |
| `pkg/ratchet/adapters` | Strict YAML loading and optional value-event history; depends on the core, with YAML confined to the loader |
| `pkg/ratchet/mcp` | Final-result SDK middleware and its consumer-owned Executor interface; no YAML or history dependency |
| `cmd/mcp-ratchet-demo` | Concrete composition, HTTP settings, startup and shutdown |

`ratchet.New(Policy, Options)` validates and clones policy, including `AfterAny`
lists. The policy is fixed for the engine's lifetime. Alternatives retain rule
and list order. The completion TTL belongs to the producer; zero policy TTL
uses five minutes. The core rejects duplicate edges, cycles, conflicting
explicit producer TTLs, and mixed unrestricted/guarded blocks.

The engine owns bounded completion stacks and session gates. One session's
check, exact selected consumption, callback, and successful recording are
serialized. A failed callback spends its reservation and grants no downstream
completion. Running and queued callers pin one entry; reclamation cannot split
a session across two gates. Idle cleanup operates on engine-owned memory and
never calls an observer or external storage while holding the global mutex.
Maintenance does not refresh the last caller's idle timestamp. Admission at
capacity also reclaims expired state.

The transport supplies isolated session IDs. There are no client credentials,
random-token ports, separate stores, policy-loader interfaces, or public
issuance/consumption methods. Applications control authentication, resource
authorization, and success semantics. Independent engines do not share state.

`Options.Observer` receives immutable value events with typed fields rather
than pointer-owned generic metadata. It is synchronous and must be safe for
concurrent calls, return promptly, and avoid same-session reentry. Optional
`adapters.MemoryEventStore` owns history and exposes its own Events/Stats/Stop;
core execution has no history proxy or event-store dependency.

`mcp.NewMiddleware` consumes only an `Executor` method accepting strings and a
success callback. It wraps the SDK's final receiving handler, snapshots all
wire JSON once, and reports success after validation and serialization. Schema,
input, handler, tool-result, metadata, and content failures cannot grant a
completion. SDK input-required results, including empty load-shedding
responses, create no completion. Each continuation RPC is gated independently;
a consumed prerequisite must be refreshed, with no pending-reservation machine.

A private result wrapper embeds a detached SDK view for sealed Result methods
and inspection, and stores every wire field as raw JSON. Its dynamic Go type is
not `*mcp.CallToolResult`; clients still decode ordinary typed SDK results. Raw
fields preserve large numbers, null/empty shapes, nested content/input-request
metadata, request state, and single-evaluation user marshalers. Only outer
metadata is rebuilt so the SDK can append server-info annotation after receiving
middleware returns; original metadata values remain raw JSON. GetError preserves
the SDK-only error cause. Output-decorating application middleware must run inside
Ratchet, before the snapshot.

The demo composes the loader, engine and SDK adapter. It accepts only literal
loopback endpoints, applies cross-origin protection, bounds idle HTTP sessions,
and drains HTTP responses and disconnected SDK calls before closing sessions.
Its POST endpoint supports SSE responses; it does not open standalone GET
streams.

Run `make ci` to enforce core import boundaries and verify direct state-machine,
adapter, two-client SDK, finalized-result, and HTTP lifecycle behavior. More
state abstraction is warranted only when a concrete shared/durable-state
requirement exists; cached event statistics need a measured query target.
