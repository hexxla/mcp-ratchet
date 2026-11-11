# Versioning and release readiness

**Maturity: alpha.** The HexxlaDB–Mosaic–MCP Ratchet stack is alpha.
No general production sign-off or beta/v1 qualification is recorded.
Existing published tags remain intact historical references.

This document owns MCP Ratchet release policy and Go core/adapter APIs, policy YAML, prerequisite consumption and SDK success semantics.

## Release policy

Versions identify releases; maturity is an explicit decision. The minor number
has no ceiling: `v0.9.0` can become `v0.10.0` and later `v0.100.0`. No amount of
iteration automatically promotes a project to v1.

- Published tags are immutable. Preserve every tag and its contents. Never delete, move, reuse or
  reset a tag to make the project look younger; retained tags also support
  reproducible dependency resolution and recovery.
- Keep versions independent across the three repositories. Record the exact
  compatible combination when qualifying the stack.
- Commit normal development without tagging every change. Publish deliberate
  milestones as annotated tags on validated commits.
- During v0, use a minor release for a feature group or intentional compatibility
  break, and patches for compatible fixes. Record breaks and migration steps
  under `CHANGELOG.md` → `[Unreleased]` before release.
- Label new development milestones explicitly, for example `v0.N.0-alpha.1`,
  then `-alpha.2`, `-beta.1` and `-rc.1`. Here `N` is a chosen future minor;
  use a numeric minor in an actual tag. A prerelease sorts below its corresponding
  normal version, so choose a base above existing normal releases.
- Move to beta, release candidate or v1 only through the gates below. A new
  feature or compatibility break during qualification requires a new candidate
  and requalification of affected behavior.
- Use Git tags for pre-v1 distribution. GitHub Release pages and automatic
  binary publication are reserved for v1 unless explicitly reconsidered.
- Follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).
  After v1, compatible additions increment minor, compatible fixes increment
  patch, and public compatibility breaks require a new major version.

## Maturity and evidence

| Stage | Meaning |
| --- | --- |
| Alpha (current) | Working software under active interface and behavior development; suitable for controlled evaluation with recoverable data. General production sign-off has not been given. |
| Beta | Intended initial scope is settled and the beta gates pass for a declared deployment profile. Changes focus on qualification and defect correction. |
| Release candidate | Intended v1 contracts and dependency versions are frozen; all v1 evidence is complete and final candidate checks pass. Only release-blocking corrections are admitted. |
| v1 | Maintainers accept the documented compatibility and support commitments for the qualified profile. Future growth remains possible under SemVer. |

CI is necessary evidence, but does not alone establish operating maturity.
Count an unrun, failed, skipped or unrecorded gate as unmet. Historical evidence
is useful background and must not be presented as a pass on a new candidate.
The [shared qualification plan](https://github.com/hexxla/mosaic/blob/main/docs/RELEASE_QUALIFICATION.md)
owns the stack workload, observation windows and report format. Run it against
explicit source SHAs and dependency versions; an unpublished local workspace
alone does not qualify the distributable stack. Repository promotion may happen
independently, but the combined stack remains alpha until its complete selected
combination passes the stack beta gates.

## Qualification progress and priorities

As of 2026-11-11, Ratchet `v0.4.0-alpha.1` passed `make ci` and `make build-all`
in the three-repository baseline. The stack recovery matrix and 30-minute /
1-hour lexical soaks also passed, including fresh-session policy denials,
receipt replay and cancellation probes; the 1-hour stage verified idle HTTP
session expiry and resumed work. Owner: void. See the
[dated stack evidence report](https://github.com/hexxla/mosaic/blob/main/docs/qualification/2026-11-11-baseline-short-soaks.md)
for exact sources, scope, results and local artifact custody.

Next priorities are to complete the 24-hour workload and representative
concurrency, custom-policy, advanced-profile and failure scenarios required by
the gates below, then the named-owner operating pilot and contract acceptance.
The current short workload is small and serial; CI and representative core
calls are not evidence that every policy/failure variant ran in the soak.
There is no new Ratchet feature commitment. Authentication, durable business
idempotency, retention and host observability remain host responsibilities;
new product features require a concrete adopting use case. Beta/v1 remain unmet.

## Beta gates

Current status: **unmet / alpha**. The smaller scope makes Ratchet a beta
candidate, but promotion requires the evidence below.

| Gate | Pass condition |
| --- | --- |
| Scope and contracts | Settle the core API, canonical YAML and MCP adapter success boundary. Declare session/TTL/state limits and observer behavior. Authentication and resource authorization remain host responsibilities. |
| Candidate checks | `make ci` and `make build-all` pass; review policy validation, cancellation around observers/reservation/dispatch, TTL expiry, exactly-once prerequisite spending per admitted call, and session cleanup. |
| Failure and isolation | Verify independent sessions, queued cancellation, failed/panicking handlers where supported, SDK input/output rejection, serialization failures, observer behavior, capacity exhaustion and process restart. Confirm no completion is issued for a failed final SDK result. |
| Integration | Test the actual pinned/publishable MCP SDK and Mosaic versions, both Mosaic profiles and custom/bundled policies. No enabled mutation may escape required protection. |
| Qualification | Complete the shared 24-hour beta workload with at least 10,000 calls including denied/failed/canceled cases; meet predeclared latency/state/memory targets and verify completion state stays bounded. |
| Review | No unresolved workflow bypass, cross-session leakage, incorrect consumption/issuance, reachable security or uncontrolled-state blocker. Document remaining limitations and ownership. |

## V1 gates

Current status: **unmet**. All beta gates must remain satisfied.

| Gate | Pass condition |
| --- | --- |
| Contract commitment | Maintainers accept core and adapter APIs, YAML, ordered alternatives, TTL semantics, cancellation/consumption behavior, events, errors and the final SDK success boundary. Document all pre-v1 migrations. |
| Operational pilot | Complete the shared 14-day named-owner host/Mosaic pilot, including policy failures, saturation/expiry, session churn and at least one host upgrade/restart. A maintainer-owned integration is sufficient. |
| Host obligations and support | Document supported SDK/toolchain/platform versions, state limits, middleware ordering, security reporting and maintenance policy. Restart clears process-local completions: require fresh prerequisites; no durable exactly-once business-write guarantee is claimed. |
| Final candidate | Freeze intended contracts/dependencies, rerun CI/vulnerability/build checks and sign off the candidate-specific integration and operating evidence. |

Ratchet's version does not certify a host's business effects. The host must
supply identity, authorization and idempotency/reconciliation of durable writes.
The combined stack remains alpha until its own selected combination qualifies.
See the [architecture contract](docs/architecture/architecture.md) and
[shared qualification plan](https://github.com/hexxla/mosaic/blob/main/docs/RELEASE_QUALIFICATION.md).
