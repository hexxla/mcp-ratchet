# MCP Ratchet overview

Ratchet enforces prerequisites between tools in one MCP session. For example,
a write may require a successful tag listing first. This makes an application's
chosen call order enforceable even when the model skips instructions.

## How a guarded call works

1. The host registers tools and installs Ratchet receiving middleware.
2. A successful prerequisite call records a server-owned completion in its session.
3. A dependent call selects an eligible prerequisite. Grouped `after_any` alternatives are
   tried in block/list order; TTL applies to the producer's completion.
4. A one-time completion is consumed before the dependent call executes.
5. Only successful execution, including SDK output validation and serialization,
   can record a downstream completion. A failed call needs a fresh one-time
   prerequisite, even if the handler already performed an external write.

There are no client Ratchet credentials. The transport supplies the
session identity. Completions cannot be borrowed from another session.

## What the application still owns

Ratchet checks ordering and tool success. The host defines authentication,
resource access, result meaning, and write-retry policy. A successful tag listing
does not prove that a later write used appropriate tags. A failed response does
not prove that an external write was rolled back; reconcile before retrying.

The engine owns process-local state. Sessions and
per-prerequisite completions are bounded; expired and idle state is reclaimed
during later activity. Optional event history is diagnostic and contains sensitive
internal identifiers, so redact it before external exposure.

See the [repository guide](../../README.md) for policy and embedding examples,
and the [architecture contract](../architecture/architecture.md) for ownership.
