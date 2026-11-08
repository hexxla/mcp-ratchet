# Project Instructions

## Architecture

- The reusable Ratchet library lives under `pkg/ratchet/`. The domain and ports are independent of adapters. Services implement the primary ports and use secondary ports for storage, clocks, and token generation.
- Keep MCP transport and tool wrapping in `pkg/ratchet/mcp/`. Keep the demo in `cmd/mcp-ratchet-demo/` and `internal/`. See the [architecture guide](docs/architecture/architecture.md) for the demo's layer boundaries.
- Preserve session isolation, prerequisite expiry, and one-time token semantics when changing tool gating. Cover denied calls and failed handlers as well as successful calls.

## Go and verification

- Use the Go version declared in `go.mod`. Follow the surrounding code and `.golangci.yml`. Wrap error causes with `%w` where callers need to inspect them.
- Run focused package tests first. Use `make test` for unit tests, `make integration` for integration tests, and `make ci` before pushing. Coverage checks use `COVERAGE_THRESHOLD` when enabled.
- Use the [repository Git hooks](.githooks/README.md) if installed. Keep commit subjects short and follow Conventional Commits.

## Mosaic integration

- Ratchet checks prerequisites for tool calls within an MCP session. Mosaic's server checks whether submitted cell kinds are allowed. Transcript capture requires an explicit policy and implementation in the host application.
- Treat conversation storage as an external write. Use Mosaic only when the task calls for it, and persist conversation content only within the host's authorized capture scope.
- For deployment behavior, see Mosaic's [Ratchet integration](https://github.com/hexxla/mosaic/blob/main/docs/mosaic/RATCHET_INTEGRATION.md) and [persistence policy](https://github.com/hexxla/mosaic/blob/main/docs/mosaic/PERSISTENCE_POLICY.md).

## Documentation

Update the owning documentation and the [changelog](CHANGELOG.md) for user-visible changes. Follow the [security policy](SECURITY.md) when handling vulnerabilities.
