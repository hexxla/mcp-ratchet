# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Common Changelog](https://common-changelog.org/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Breaking

- Consolidate the reusable API in `pkg/ratchet`: `New(Policy, Options)` returns an Engine that owns bounded in-memory completion state. Remove superseded domain/services/ports packages, token/random/clock adapters, registration/reload methods, and construction/storage interfaces. There are no compatibility wrappers.
- Replace individual prerequisite rules with ordered `Rule{Tool, AfterAny, Consume, TTL, Message}` blocks and `Policy.CompletionTTL`. Canonical YAML uses `completion_ttl`, `after_any`, `consume`, `ttl`, and `message`; reject legacy keys and inert observability configuration. TTL belongs to the producer and defaults to five minutes.
- Replace pointer events, opaque tokens and arbitrary metadata with immutable typed Event values. Observe them through `Options.Observer`; optional `adapters.MemoryEventStore` independently exposes Observe/Events/Stats/Stop. Engine execution has no history-query proxy.
- Replace typed `WrapWithRatchet` with `mcp.NewMiddleware`, installed once using `Server.AddReceivingMiddleware`. Its consumer-owned Executor interface uses strings; register tools normally. Executed results have a private snapshot-wrapper Go type implementing SDK Result, while clients retain ordinary typed results. Application middleware decorating output must run inside Ratchet. The demo refuses standalone GET streams with 405; POST SSE responses remain supported.

### Fixed

- Grant prerequisites only after SDK output validation and serialization succeed. SDK 1.7 input-required and load-shedding responses create no completion; continuation RPCs need fresh consumed prerequisites. Snapshot every wire field, including input requests and request state, while preserving large JSON numbers, null/empty shapes, metadata, single-evaluation user marshalers, SDK error causes, and SDK server-info decoration.
- Preserve exact one-time spending before calls, tool-error responses, queued cancellation, failed-handler denial, and session isolation. Reclaim expired idle state before rejecting new sessions at capacity; maintenance does not extend idle lifetime.
- Eliminate event-history aliasing and statistics races through immutable value events and detached query slices/maps.
- Reject cross-origin demo mutations and nonliteral endpoint paths, expire idle HTTP sessions after 30 minutes, and drain active calls (including disconnected SDK calls) before closing MCP sessions at shutdown.
- Replace placeholder security and conduct contacts, update supported versions, and correct stale protocol, architecture, release-check, and changelog-link guidance.

### Changed

- Consolidate architecture guidance into the project-specific boundary contract; remove the superseded implementation plan and generic layer tutorials.
- Enforce a standard-library-only core and separate MCP/YAML adapter boundaries. Keep the existing CI pipeline and simple historical statistics scan.
- Pin the MCP Go SDK to v1.7.0, matching the Mosaic integration baseline.

## [0.3.0] - 2026-11-09

### Fixed

- Release session slots when calls finish without live prerequisites. Expired prerequisites no longer keep idle sessions resident until the idle timeout.
- Use live prerequisite tokens as the executor's completion state and avoid a duplicate session history.

### Changed

- Add a write-only `EventSink` for live observability. Historical event stores still support queries.
- Replace separate validation, consumption, issuance, and session creation with `ExecuteToolCall`. Service constructors no longer accept a session store.
- Custom token stores must implement `HasLiveTokens`. The unused event-store factory and broadcasting adapter were removed.

## [0.2.0] - 2026-11-09

### Workflow execution

- Bound in-memory workflow sessions and completions, reclaim idle state, and
  serialize only calls in the same session. The built-in token store drops
  expired completions on lookup.
- Reject unknown YAML fields, extra documents, duplicate edges, cycles, mixed
  guarded and unguarded rules, and conflicting producer expiry values.
- Remove the demo's HTTP and WebSocket observability and the standalone
  WebSocket test command.

### API changes

- `WrapWithRatchet` takes the workflow executor directly. The service
  constructors return the concrete service so it can be passed without a cast.
- Custom token stores now provide `LatestToken` and `DeleteSession` for expiry
  checks and cleanup.

### Added

- Initial project structure with hexagonal architecture (domain, port, service, adapter)
- Makefile with common development targets
- CI pipeline with formatting, linting, tests, and architecture guardrails
- Comprehensive documentation for each architectural layer

### Changed

- **Development workflow** — Validate pull requests and pushes to `main` with the shared CI check and narrow routine lint checks.

### Fixed

- Isolate MCP calls by transport session and reserve the exact matching one-time prerequisite before the handler. Return detached in-memory session snapshots.

- Make in-memory event store shutdown safe for concurrent callers and release references to expired events during pruning.

### Changed

- Keep the demo in its command package, bind it to loopback, and redact event tokens and metadata.

- Generate observability event IDs with the standard library's UUID v4 generator.

- **Go toolchain** — require Go 1.27.3 or newer. GitHub Actions uses the version in `go.mod`.

- **CI and releases** — keep the demo binary as a CI build check. Tags are published manually from validated commits.

- Use repository-native Codex instructions for development. Removed the legacy assistant setup and cleanup targets, generated workflows, and automatic transcript capture guidance.
- Updated folder structure to clearly separate primary and secondary ports/adapters

## [0.1.0] - 2025-04-25

### Added

- Bootstrapper CLI skeleton (`cmd/mcp-ratchet-demo/main.go`)
- Hexagonal architecture guardrail script
- AGENTS.md with instructions for LLMs and contributors
- Layer-specific README.md files explaining responsibilities

### Changed

- Standardized project layout for LLM-friendly hexagonal architecture templates

[Unreleased]: https://github.com/hexxla/mcp-ratchet/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/hexxla/mcp-ratchet/tree/v0.3.0
[0.2.0]: https://github.com/hexxla/mcp-ratchet/releases/tag/v0.2.0
[0.1.0]: https://github.com/hexxla/mcp-ratchet/releases/tag/v0.1.0
