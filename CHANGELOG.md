# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Common Changelog](https://common-changelog.org/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial project structure with hexagonal architecture (domain, port, service, adapter)
- Makefile with common development targets
- CI pipeline with formatting, linting, tests, and architecture guardrails
- Comprehensive documentation for each architectural layer

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

[Unreleased]: https://github.com/hexxla/mcp-ratchet/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/hexxla/mcp-ratchet/releases/tag/v0.1.0
