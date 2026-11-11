# Project Instructions

## Architecture

- The public core is `github.com/hexxla/mcp-ratchet/pkg/ratchet`: Policy, Rule, Event and Engine. Keep it independent of YAML, MCP SDK, concrete adapters and storage abstractions.
- Keep canonical YAML loading and optional event history in `pkg/ratchet/adapters`, SDK middleware and its consumer-owned Executor interface in `pkg/ratchet/mcp`, and composition/lifecycle in `cmd/mcp-ratchet-demo`. See the [architecture guide](docs/architecture/architecture.md).
- Preserve session isolation, completion expiry, exact consumption before calls, per-session cancellation and bounded state. Cover denied calls, failed handlers and final SDK output failures as well as success.
- Events are typed values with no token or generic metadata. Keep history independent of the engine and observer callbacks outside global bookkeeping locks.

## Go and verification

- Use the Go version declared in `go.mod`. Follow the surrounding code and `.golangci.yml`. Wrap error causes with `%w` where callers need to inspect them.
- In Codex, use the [JetBrains Modern Go Guidelines plugin](https://github.com/JetBrains/go-modern-guidelines#codex) when available.
- Run focused package tests first. Use `make test` for race tests and `make ci` before pushing.
- The [optional Git hook](.githooks/README.md) scans staged changes for secrets. Keep commit subjects short and follow Conventional Commits.

## Mosaic integration

- Ratchet checks prerequisites for tool calls within an MCP session. Mosaic's server checks whether submitted cell kinds are allowed. Transcript capture requires an explicit policy and implementation in the host application.
- Treat conversation storage as an external write. Use Mosaic only when the task calls for it, and persist conversation content only within the host's authorized capture scope.
- For deployment behavior, see Mosaic's [Ratchet integration](https://github.com/hexxla/mosaic/blob/main/docs/mosaic/RATCHET_INTEGRATION.md) and [persistence policy](https://github.com/hexxla/mosaic/blob/main/docs/mosaic/PERSISTENCE_POLICY.md).

## Documentation

Update the owning documentation and the [changelog](CHANGELOG.md) for user-visible changes. Follow the [security policy](SECURITY.md) when handling vulnerabilities.

## Release maturity

The repository and combined stack are alpha. Follow [VERSIONING.md](VERSIONING.md):
preserve published tags, keep versions independent, publish deliberate v0
prerelease milestones and require recorded beta/v1 qualification before promotion.
