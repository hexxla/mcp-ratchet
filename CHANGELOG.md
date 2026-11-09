# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Common Changelog](https://common-changelog.org/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

- **Development workflow** — Use the CI check on `main` to validate version tags and narrow routine lint checks.

### Fixed

- Isolate MCP calls by transport session and reserve the exact matching one-time prerequisite before the handler. Return detached in-memory session snapshots.

- Make in-memory event store shutdown safe for concurrent callers and release references to expired events during pruning.

### Changed

- Keep the demo in its command package, bind it to loopback, and redact event tokens and metadata.

- Generate observability event IDs with the standard library's UUID v4 generator.

- **Go toolchain** — require Go 1.27.3 or newer. GitHub Actions uses the version in `go.mod`.

- **CI and releases** — validate Go module tags and keep the demo binary as a CI build check.

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

[Unreleased]: https://github.com/hexxla/mcp-ratchet/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/hexxla/mcp-ratchet/releases/tag/v0.2.0
[0.1.0]: https://github.com/hexxla/mcp-ratchet/releases/tag/v0.1.0
