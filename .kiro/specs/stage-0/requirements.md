# Requirements: Millefeuille Stage 0 (M0.1) — Core Observation Storage

## Metadata

- Status: Approved
- Owner: habuka036 (Project Maintainer)
- Created: 2026-10-10
- Updated: 2026-10-10 (DD-002 resolved: received_at assigned by library)

## Context

Millefeuille is an independent, reusable memory architecture for evolving AI agents.
It provides policy-aware, multi-layered storage for observations received from external
devices, agents, simulators, and other sources.

Stage 0 (M0.1) establishes the foundational persistence layer: a Go library that durably
appends, retrieves, and queries observations backed by SQLite. No HTTP server, gRPC
server, LLM calls, vector search, or memory promotion are included in this stage.

The library will be consumed by IMAGO Agent (HTTP Gateway layer) and governed by Guardian
(authorization and audit layer). Millefeuille itself enforces no authentication or
authorization in Stage 0; those concerns belong to the caller.

## Goals

- Provide a Go library API (`AppendObservation`, `GetObservation`, `QueryObservations`) for durable observation storage.
- Persist observations to SQLite with full transactional guarantees.
- Support idempotent re-append of identical observations and reject conflicting duplicates.
- Enforce append-only semantics; no observation may be overwritten after storage.
- Expose context-aware APIs that respect `context.Context` cancellation and deadlines.
- Enable bounded queries filtered by `source_id`, `event_type`, and `received_at` time range with a configurable limit.
- Record provenance and visibility metadata alongside each observation without interpreting payload semantics.
- Provide a database schema migration mechanism for forward compatibility.

## Non-goals

- HTTP, gRPC, or any network transport layer.
- Authentication, authorization, or capability enforcement.
- Payload semantic interpretation, LLM calls, or embedding generation.
- Vector search or similarity-based retrieval.
- Memory promotion or knowledge inference.
- Cursor-based pagination (deferred to M0.2).
- Multi-database or non-SQLite storage backends in Stage 0.
- Observation update or deletion APIs.

## Assumptions

- The caller (e.g., IMAGO Agent) is responsible for authenticating the external source before invoking Millefeuille APIs.
- `received_at` is assigned internally by Millefeuille after input validation passes and the observation is accepted for storage (UTC). The caller does not supply `received_at` on new appends; the library sets it, persists it, and returns it in the `Receipt` and in subsequent reads.
- `visibility` and `provenance` are set by the trusted caller; Millefeuille applies defaults (`visibility: "private"`, `trust_level: "unverified"`) when not explicitly provided.
- `event_id` is generated and guaranteed unique by the source or gateway before submission.
- UTC is the canonical timezone for all timestamps stored and returned by the library.
- Stage 0 supports a single application process with a single logical writer. Concurrent readers within that process are permitted. Multi-process coordination is outside M0.1 scope.
- SQLite uses WAL journal mode, with explicit durability and checkpoint behavior documented in detailed design.
- `trust_level` in provenance reflects the caller's assessment of the source; Millefeuille stores it as-is without validation.

## Constraints

- Implementation language: Go; prefer Go standard library. Introduce third-party dependencies only when justified.
- Storage: SQLite via a Go SQLite driver. All writes use database transactions.
- JSON payload size limit: 1 MiB per observation.
- All SQL operations must use parameterized queries; string interpolation into SQL is prohibited.
- All timestamps are stored and returned in UTC.
- The library must not import packages that introduce HTTP or gRPC server dependencies.
- Schema changes must be managed through versioned migrations; direct DDL execution outside migrations is prohibited.
- M0.1 SHALL NOT create an index on `device_id`; adding `device_id` filtering/indexing is deferred to M0.2 or later.
- `schema_version` SHALL be stored as a non-empty free-form string without allowlist/version compatibility validation in M0.1.
- Required string fields SHALL be non-empty in M0.1; maximum lengths SHALL be determined in detailed design before implementation.
- `received_at` SHALL be assigned by the library (not the caller) after input validation passes and the observation is accepted for storage, in UTC. Callers SHALL NOT supply `received_at` in the input `Observation`; the field is absent from the input type or ignored if present.
- If a gateway-side receipt timestamp is required in the future, it SHALL be recorded as a separate field (e.g., `gateway_received_at`) and SHALL NOT alter the semantics of `received_at`.
- The time-acquisition function used to assign `received_at` SHALL be injectable for testing purposes; production code SHALL use the real system clock.

---

## Requirements

### REQ-001: AppendObservation — durable append

**User story**

As a caller (e.g., IMAGO Agent),
I want to append a new observation to durable storage,
so that the event is persisted and can be retrieved later.

**Acceptance criteria**

1. WHEN a valid `Observation` is submitted via `AppendObservation`
   THE SYSTEM SHALL assign `received_at` to the current UTC time after input validation passes and the observation is accepted for storage, persist the observation to SQLite within a transaction, and return a `Receipt` containing the `event_id`, `received_at`, and a stable `content_hash`.

2. WHEN `AppendObservation` completes successfully
   THE SYSTEM SHALL guarantee the observation is durably written such that it survives a process restart.

3. WHEN the `context.Context` passed to `AppendObservation` is cancelled or has exceeded its deadline before the write completes
   THE SYSTEM SHALL abort the operation and return a context error without partially persisting data.

4. IF the observation payload exceeds 1 MiB
   THEN THE SYSTEM SHALL reject the call and return an `ErrPayloadTooLarge` error without writing any data.

5. IF any required caller-provided field (`event_id`, `schema_version`, `source_id`, `event_type`, `observed_at`, `payload`, `provenance.origin`) is missing or empty
   THEN THE SYSTEM SHALL reject the call and return an `ErrInvalidObservation` error describing the missing field.
   (`received_at` is library-assigned and SHALL NOT be required from the caller.)

---

### REQ-002: AppendObservation — idempotent re-append

**User story**

As a caller,
I want identical re-submissions of the same observation to succeed without duplication,
so that transient network retries do not produce duplicate records.

**Acceptance criteria**

1. WHEN `AppendObservation` is called with an `event_id` that already exists in storage AND the normalized content of the new observation matches the stored observation
   THE SYSTEM SHALL return the original `Receipt` without inserting a duplicate record.

2. WHEN `AppendObservation` is called with an `event_id` that already exists in storage AND the normalized content of the new observation differs from the stored observation
   THE SYSTEM SHALL return an `ErrConflict` error and leave the stored observation unchanged.

3. WHEN computing the match for idempotency
   THE SYSTEM SHALL compare a canonicalized immutable event identity/content projection using a stable hash computed at storage time. This projection SHALL explicitly exclude `received_at` (library-assigned) and SHALL include: `event_id`, `schema_version`, `source_id`, `device_id`, `event_type`, `observed_at`, `payload`, `visibility`, and `provenance`. Retries that re-submit identical caller-supplied fields SHALL NOT be misclassified as conflicts due to a differing `received_at`.

4. WHEN `AppendObservation` detects a duplicate by idempotency match
   THE SYSTEM SHALL return the original `Receipt` including the originally stored `received_at`, without issuing a new timestamp.

---

### REQ-003: GetObservation — single event retrieval

**User story**

As a caller,
I want to retrieve a specific observation by its `event_id`,
so that I can inspect or verify a previously stored event.

**Acceptance criteria**

1. WHEN `GetObservation` is called with a valid `event_id` that exists in storage
   THE SYSTEM SHALL return the complete `Observation` for that `event_id`.

2. WHEN `GetObservation` is called with an `event_id` that does not exist in storage
   THE SYSTEM SHALL return `ErrNotFound` as a sentinel error so that callers can detect the not-found condition using `errors.Is(err, ErrNotFound)`.

3. WHEN the `context.Context` passed to `GetObservation` is cancelled or has exceeded its deadline
   THE SYSTEM SHALL abort the read and return a context error.

---

### REQ-004: QueryObservations — bounded query

**User story**

As a caller,
I want to query observations by filter criteria with a bounded result set,
so that I can retrieve relevant events without loading unbounded data.

**Acceptance criteria**

1. WHEN `QueryObservations` is called with a `Filter`
   THE SYSTEM SHALL return a `Page` containing at most `limit` observations, ordered by `received_at DESC, event_id DESC`.

2. WHEN `Filter.limit` is not specified or is zero
   THE SYSTEM SHALL apply a default limit of 100.

3. WHEN `Filter.limit` exceeds 1000
   THE SYSTEM SHALL cap the effective limit at 1000 and proceed without error.

4. WHEN `Filter.source_id` is specified
   THE SYSTEM SHALL return only observations whose `source_id` matches exactly.

5. WHEN `Filter.event_type` is specified
   THE SYSTEM SHALL return only observations whose `event_type` matches exactly.

6. WHEN `Filter.time_range` (`from`, `to`) is specified
   THE SYSTEM SHALL return only observations where `received_at` falls within the inclusive range `[from, to]`.

7. WHEN multiple filter fields are specified
   THE SYSTEM SHALL apply all conditions with logical AND.

8. WHEN no observations match the filter
   THE SYSTEM SHALL return an empty `Page` with zero items and an empty `next_cursor`.

9. WHEN `QueryObservations` returns a `Page`
   THE SYSTEM SHALL set `next_cursor` to an empty string in M0.1 regardless of whether more results exist beyond the limit.

10. WHEN the `context.Context` passed to `QueryObservations` is cancelled or has exceeded its deadline
    THE SYSTEM SHALL abort the query and return a context error.

---

### REQ-005: Observation data model

**User story**

As a developer integrating Millefeuille,
I want a well-defined `Observation` struct,
so that I can construct and interpret observations without ambiguity.

**Acceptance criteria**

1. WHEN an `Observation` is stored or returned
   THE SYSTEM SHALL include all of the following fields:

   | Field            | Type      | Caller-supplied | Description |
   |------------------|-----------|-----------------|-------------|
   | `event_id`       | string    | Yes             | Unique identifier; UUID or ULID |
   | `schema_version` | string    | Yes             | Input schema version; initial value `"0.1"` |
   | `source_id`      | string    | Yes             | Identifier of the observation origin |
   | `device_id`      | string    | Optional        | Physical device identifier; may be empty |
   | `event_type`     | string    | Yes             | Event kind, e.g. `"device.status"` |
   | `observed_at`    | timestamp | Yes             | Time of occurrence as reported by the source (UTC) |
   | `received_at`    | timestamp | No (library-assigned) | UTC time at which `AppendObservation` accepted the call; absent on input, set on output |
   | `payload`        | JSON      | Yes             | Opaque observation content; max 1 MiB |
   | `visibility`     | string    | Optional        | Disclosure scope; defaults to `"private"` if not provided |
   | `provenance`     | object    | Yes (`origin` required) | Origin and trust metadata; `trust_level` defaults to `"unverified"` |

2. WHEN `provenance` is stored or returned
   THE SYSTEM SHALL include at minimum:

   | Field         | Type   | Description |
   |---------------|--------|-------------|
   | `origin`      | string | Source category: `device`, `agent`, `simulator`, `external` |
   | `trust_level` | string | Caller's trust assessment; default `"unverified"` |

3. WHEN `received_at` is set
   THE SYSTEM SHALL assign it internally after input validation passes and the observation is accepted for storage (not at DB commit time), in UTC. The caller SHALL NOT provide `received_at`; it SHALL be absent from the input type or ignored if present. The assigned value SHALL be persisted and returned unchanged in the `Receipt` and in all subsequent reads, including idempotent re-appends.

4. WHEN the time-acquisition function is designed
   THE SYSTEM SHALL make the clock injectable (e.g., via a function parameter or struct field) to enable deterministic testing, without introducing unnecessary abstraction in production paths.

5. WHEN `schema_version` is provided
   THE SYSTEM SHALL require a non-empty string but SHALL NOT validate it against an allowlist in M0.1.

6. WHEN required string identifiers are validated
   THE SYSTEM SHALL reject empty strings; explicit maximum lengths SHALL be decided in detailed design before implementation.

---

### REQ-006: Append-only semantics

**User story**

As a system architect,
I want observations to be immutable once stored through the library's API,
so that the record of what was observed cannot be altered or removed via Millefeuille's public interface.

**Acceptance criteria**

1. WHEN any observation has been successfully persisted
   THE SYSTEM SHALL provide no API to update or delete that observation in Stage 0.

2. WHEN the storage layer is queried for a previously stored observation
   THE SYSTEM SHALL return the observation with the exact content recorded at insertion time.

---

### REQ-007: Schema migration

**User story**

As an operator,
I want the database schema to be managed through versioned migrations,
so that the database can be upgraded without manual DDL.

**Acceptance criteria**

1. WHEN the library is initialized against an empty or existing SQLite database
   THE SYSTEM SHALL apply any pending migrations in order and bring the schema to the current version.

2. WHEN a migration has already been applied to the database
   THE SYSTEM SHALL skip that migration and not re-apply it.

3. WHEN a migration fails
   THE SYSTEM SHALL roll back the failed migration transaction and return an error without leaving the schema in a partially migrated state.

---

## Non-functional requirements

### NFR-000: SQLite journal mode

1. WHEN the SQLite database is initialized
   THE SYSTEM SHALL configure and verify `PRAGMA journal_mode=WAL` for a file-backed database, and fail initialization if the required mode cannot be established.

2. WHEN SQLite connection and transaction settings are designed
   THE SYSTEM SHALL document and test `busy_timeout`, transaction concurrency, checkpoint handling, and `synchronous` mode. M0.1 SHALL target crash-safe committed writes with `synchronous=FULL` unless the detailed design documents and obtains approval for a different durability tradeoff.

3. WHEN the database is opened
   THE SYSTEM SHALL ensure per-connection PRAGMAs are applied to each active connection as appropriate; journal mode persistence alone SHALL NOT be assumed to configure every connection.


### NFR-001: Transactional write integrity

1. WHEN `AppendObservation` writes to SQLite
   THE SYSTEM SHALL execute all inserts within a single database transaction so that a crash mid-write leaves no partial record.

### NFR-002: Context propagation

1. WHEN any API method receives a `context.Context` that is cancelled or past its deadline
   THE SYSTEM SHALL surface the context error to the caller promptly without completing the underlying database operation.

### NFR-003: SQL injection prevention

1. WHEN any SQL statement is executed
   THE SYSTEM SHALL use parameterized queries exclusively; no user-supplied or computed value SHALL be interpolated directly into a SQL string.

### NFR-004: UTC timestamp storage

1. WHEN any timestamp is written to or read from the database
   THE SYSTEM SHALL normalize the value to UTC so that all persisted timestamps are timezone-consistent.

### NFR-005: Payload size enforcement

1. WHEN `AppendObservation` receives a payload
   THE SYSTEM SHALL measure the serialized byte length before writing and reject payloads exceeding 1,048,576 bytes (1 MiB).

---

## Compatibility requirements

### COMP-001: Forward-compatible Page type

1. WHEN `QueryObservations` returns a `Page`
   THE SYSTEM SHALL include a `next_cursor` field of type string in the response struct so that M0.2 cursor-based pagination can be added without a breaking API change.

### COMP-002: Adapter-ready interface

1. WHEN the public Go interface for `AppendObservation`, `GetObservation`, and `QueryObservations` is defined
   THE SYSTEM SHALL define it as a Go interface (not a concrete struct) so that HTTP or gRPC adapter layers can wrap it without modifying the core library.

---

## Security and privacy requirements

### SEC-001: Private visibility default

1. WHEN an observation is stored without an explicit `visibility` value
   THE SYSTEM SHALL default `visibility` to `"private"`.

2. WHEN an observation with `visibility: "private"` is stored
   THE SYSTEM SHALL NOT enforce access control itself, but SHALL preserve the visibility label faithfully so that the hosting application can enforce authorization at its boundary.

### SEC-002: Trust level default

1. WHEN an observation is stored without an explicit `trust_level` in `provenance`
   THE SYSTEM SHALL default `trust_level` to `"unverified"`.

2. WHEN `trust_level` is provided by an untrusted input source
   THE SYSTEM SHALL require the trusted caller to explicitly set this field; Millefeuille SHALL NOT infer or elevate trust automatically.

### SEC-003: Parameterized queries

1. WHEN constructing any database query
   THE SYSTEM SHALL use parameterized statements to prevent SQL injection; direct string concatenation of user input into SQL is prohibited.

---

## Resolved questions (pending requirements approval)

- **OQ-001 — Resolved**: No `device_id` index or filter in M0.1. Revisit in M0.2+.
- **OQ-002 — Resolved**: Use SQLite WAL mode. Exact connection, checkpoint and durability settings are design tasks subject to NFR-000.
- **OQ-003 — Resolved**: Store non-empty `schema_version` as a free-form string; no version allowlist validation in M0.1.
- **OQ-004 — Resolved for requirements**: Require non-empty strings; decide concrete maximum lengths during detailed design before implementation.
- **DD-002 — Resolved**: `received_at` is assigned by Millefeuille library after input validation passes and the observation is accepted for storage (UTC), not by the caller or gateway. If a gateway-side timestamp is needed in the future, it will be a separate field (e.g., `gateway_received_at`). The clock source SHALL be injectable for testing. `received_at` represents the library's acceptance time after validation, not DB commit time.

## Design-phase decisions to review

- **DD-001**: Define canonical idempotency projection fields and confirm whether `visibility` and `provenance` participate in the hash.
- **DD-003**: Define SQLite connection pool and WAL checkpoint behavior, including backup and shutdown procedure.

> **Approval gate:** Requirements approved by habuka036 on 2026-10-10. Proceeding to design phase.
