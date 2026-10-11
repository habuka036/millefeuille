# Design: Millefeuille Stage 0 (M0.1) — Core Observation Storage

## Metadata

- Status: Approved
- Related spec: `.kiro/specs/stage-0/requirements.md`
- Created: 2026-10-10
- Updated: 2026-10-10 (revision 5: internal/model package introduced to resolve import cycle; domain types moved and re-exported as aliases; public API unchanged)

---

## Requirement mapping

| Requirement | Design section | Verification |
|---|---|---|
| REQ-001 | Proposed design — AppendObservation flow; Components — Store; Data flow | Unit test: durable append |
| REQ-002 | Proposed design — Idempotency; Components — Store | Unit test: idempotent re-append, conflict |
| REQ-003 | Proposed design — GetObservation flow; Components — Store | Unit test: found / not-found |
| REQ-004 | Proposed design — QueryObservations flow; Components — Store | Unit test: filter combinations, limit capping |
| REQ-005 | Data-model changes — Go types | Unit test: field presence, defaults |
| REQ-006 | Proposed design — Append-only enforcement | Unit test: no update/delete API exposed |
| REQ-007 | Components — Migrator; Data-model changes — Schema | Migration test: apply, idempotent skip, rollback |
| NFR-000 | Components — DB initializer; SQLite connection management; Error handling — WAL setup failure | Integration test: WAL mode verified on open |
| NFR-001 | Proposed design — AppendObservation flow (transaction) | Unit test: crash-safe write |
| NFR-002 | Proposed design — Context propagation and transaction semantics | Unit test: cancelled context returns ctx error |
| NFR-003 | Security considerations — Parameterized queries | Code review; unit test: no interpolation |
| NFR-004 | Data-model changes — Timestamp storage | Unit test: UTC roundtrip, nanosecond precision |
| NFR-005 | Proposed design — Validation; Error handling | Unit test: payload > 1 MiB rejected |
| COMP-001 | Data-model changes — Page type | Unit test: next_cursor always empty in M0.1 |
| COMP-002 | Components — Store interface | Compile-time check: interface satisfied |
| SEC-001 | Proposed design — Default application | Unit test: visibility defaults to "private" |
| SEC-002 | Proposed design — Default application | Unit test: trust_level defaults to "unverified" |
| SEC-003 | Security considerations — Parameterized queries | Code review; static analysis |

---

## Current architecture

The repository (`millefeuille`) is a new, empty Go project. There is no existing application
code, storage layer, or migration infrastructure. The design described here establishes the
initial architecture from scratch.

External context:
- **IMAGO Agent** will call this library as its memory back-end. It owns the HTTP Gateway,
  device communication, and input validation.
- **Guardian** will wrap or gate calls to enforce capability authorization and audit policy.
- Millefeuille itself has no knowledge of either component and must not import them.

---

## Proposed design

### Overview

Millefeuille exposes a single public Go interface (`Store`) backed by a SQLite repository.
A thin `store` struct wires the repository with default settings (clock, limits). The caller
constructs a `Store` via `Open(ctx, path, opts)` which runs migrations and verifies WAL mode
before returning.

```
Caller (IMAGO Agent)
      │
      ▼
  Store interface          ← public API; defined in package millefeuille
      │                       (domain types re-exported as aliases of internal/model)
      ▼
  store struct             ← wires repo + clock + config
      │
      ▼
  ObservationRepository    ← interface; SQL operations only (uses internal/model types)
      │
      ▼
  sqliteRepository         ← implements ObservationRepository using *sql.DB
      │
      ▼
  SQLite (WAL mode, file-based only)
```

The `Migrator` is invoked once during `Open` and is not part of the `Store` interface.

---

### Package layout

```
millefeuille/                  ← module root (github.com/habuka036/millefeuille)
  store.go                     ← Store interface, Open(), store struct, Option
  observation.go               ← type aliases re-exporting model types (public API)
  errors.go                    ← sentinel errors (ErrNotFound, ErrConflict, ErrInvalidObservation, ErrPayloadTooLarge, ErrInvalidFilter)
  clock.go                     ← ClockFunc type; default = time.Now
  internal/
    model/
      model.go                 ← domain types: Observation, InputObservation, Provenance, Receipt, Filter, Page
    sqlite/
      repository.go            ← sqliteRepository (implements ObservationRepository)
      migrator.go              ← Migrator, migration table, versioned SQL
      migrations/
        001_initial_schema.sql ← DDL for observations table
    hash/
      hash.go                  ← canonical idempotency hash computation
```

**Dependency direction** (no cycles):

```
millefeuille (root)  ──imports──►  internal/model
       │                                 ▲
       ├──imports──► internal/sqlite ────┤
       └──imports──► internal/hash ──────┘
```

- `internal/model` has no dependencies on any other package in this module.
  It defines the domain types only.
- `internal/sqlite` and `internal/hash` import `internal/model` but never the
  root `millefeuille` package. This prevents the import cycle that would arise
  if domain types lived in the root package (the root imports the internal
  packages via `Open`).
- The root `millefeuille` package re-exports the domain types with type aliases
  (`type Observation = model.Observation`, etc.) so the public API is unchanged.
- `ClockFunc` remains in the root package as it is part of the `Store`
  configuration surface (via `WithClock`), not a persisted domain type.

All types under `internal/` are inaccessible to callers of the module. The
public surface is `store.go`, `observation.go` (aliases), `errors.go`, and
`clock.go`.

---

### AppendObservation flow

1. Validate required caller-supplied fields; return `ErrInvalidObservation` on failure.
2. Validate `payload` with `json.Valid`; return `ErrInvalidObservation` if payload is not valid JSON.
3. Enforce payload size ≤ 1 MiB (measured as byte length of the input JSON); return `ErrPayloadTooLarge` if exceeded.
4. Apply defaults: `visibility = "private"` if empty; `provenance.trust_level = "unverified"` if empty.
5. Capture `received_at = clock().UTC()` after validation passes. This is the acceptance timestamp (post-validation), not the DB commit time.
6. Compute `content_hash` over the canonical idempotency projection (see Idempotency section).
7. Begin SQLite transaction.
8. Attempt `INSERT INTO observations … ON CONFLICT(event_id) DO NOTHING` and check rows affected.
   - Rows affected = 1 → new insert; commit and return `Receipt`.
   - Rows affected = 0 → duplicate `event_id` exists; read stored `content_hash`.
     - Hashes match → idempotent success; rollback (nothing to commit); return original `Receipt`.
     - Hashes differ → rollback; return `ErrConflict`.
9. On any DB or context error → rollback; return error.

> **Why `ON CONFLICT DO NOTHING` + hash check rather than `INSERT OR REPLACE`?**
> `INSERT OR REPLACE` would silently overwrite, violating append-only semantics (REQ-006).
> The two-step approach is explicit and preserves the original `received_at`.

---

### Idempotency and content hash (DD-001 resolved)

The canonical projection included in the hash is:

| Field | Included | Rationale |
|---|---|---|
| `event_id` | Yes | Identity anchor |
| `schema_version` | Yes | Part of event identity |
| `source_id` | Yes | Part of event identity |
| `device_id` | Yes | Part of event identity (empty string `""` if absent) |
| `event_type` | Yes | Part of event identity |
| `observed_at` | Yes | Source-reported time; caller-supplied; serialised as nanosecond-precision UTC string |
| `payload` | Yes | Core content |
| `visibility` | Yes | Caller-assessed; change = different intent |
| `provenance.origin` | Yes | Caller-assessed; change = different trust claim |
| `provenance.trust_level` | Yes | Caller-assessed; change = different trust claim |
| `received_at` | **No** | Library-assigned; excluded to allow safe retries |

**Hash algorithm**: SHA-256 over a canonical JSON serialisation of the above fields.

**Canonicalisation rules** (applied before hashing; these are the normative rules that
determine whether two submissions are considered identical):

1. The canonical projection struct is marshalled to JSON using Go's `encoding/json`.
2. `observed_at` is serialised as a nanosecond-precision UTC string
   (format: `2006-01-02T15:04:05.000000000Z`).
3. `device_id` is included as empty string `""` when absent.
4. `payload` is normalised through a decode-then-encode cycle to produce a canonical form:
   - Decode the raw JSON bytes using `json.NewDecoder` with `UseNumber()` enabled, into `any`.
     `UseNumber()` preserves numeric values as `json.Number` strings rather than converting
     them to `float64`, preventing precision loss.
   - Re-encode with `json.Marshal`. Because Go's `encoding/json.Marshal` sorts map keys
     lexicographically at every level of nesting, the resulting JSON is key-order-normalised
     throughout the entire payload tree.
   - This means: two `payload` values that are semantically equivalent but differ only in
     key order or whitespace at any nesting level will produce the **same** canonical bytes
     and therefore the **same** hash.
5. All other projection fields are plain strings or simple structs marshalled with standard
   `json.Marshal`; key order in the outer projection struct is fixed by the Go struct field
   order (or an explicit sorted marshalling step).
6. No trailing whitespace or newlines.

**Scope of identity guarantee**: two `AppendObservation` calls are considered identical
(idempotent retry) if and only if all fields in the canonical projection above produce the
same SHA-256 digest. The normalisation handles whitespace differences and key-order
differences at all levels of `payload`. Numeric representation differences (e.g., `1` vs
`1.0` vs `1e0`) are handled consistently through `json.Number` preservation: `1` and `1.0`
are distinct JSON numbers and will remain distinct after the decode/encode cycle, producing
different hashes. Callers should submit numerically consistent payloads for retries.

Duplicate keys in `payload` JSON: the raw input bytes are stored as-is in the database
(including any duplicate keys). For hash computation only, the payload is decoded with
`UseNumber()` — Go's decoder applies last-key-wins semantics when the same key appears
multiple times — and then re-encoded, which yields a deduplicated canonical form. As a
result, two payloads that differ only in duplicate key repetition but share the same
last-key-wins values will produce the **same hash** and be treated as idempotent. Callers
should not submit payloads with duplicate keys; the stored raw bytes will contain the
duplicates, but hash comparison ignores them.

The resulting hex digest is stored in the `content_hash` column and returned in
`Receipt.ContentHash`.

**Required test cases for canonicalisation**:
- Two submissions with identical fields → same hash.
- Two submissions with `payload` differing only in whitespace at any level → same hash.
- Two submissions with `payload` differing only in key order at any nesting level → same hash.
- Two submissions with `payload` differing in numeric representation (`1` vs `1.0`) →
  different hash; test must confirm and document this behaviour.
- Two submissions with `payload` containing duplicate keys → same hash after last-key-wins
  resolution; test must confirm determinism.

---

### GetObservation flow

1. Execute `SELECT … FROM observations WHERE event_id = ?` with the given `event_id`.
2. Row found → map columns to `Observation`; return `(obs, nil)`.
3. Row not found → return `(nil, ErrNotFound)`.
4. Context cancelled before or during query → return `(nil, ctx.Err())`.

> **NotFound as error sentinel**: `ErrNotFound` is a typed sentinel
> (`var ErrNotFound = errors.New("observation not found")`), not a second return value,
> so callers can use `errors.Is`. This is consistent with Go idiom.

---

### QueryObservations flow

1. Validate `Filter.Limit`: if negative, return `ErrInvalidFilter` immediately.
2. Normalise limit: `0 → 100`, `> 1000 → 1000`.
2. Build parameterised SQL dynamically:
   - Start with an empty conditions slice and a params slice.
   - Append `source_id = ?` + value if `Filter.SourceID` is non-empty.
   - Append `event_type = ?` + value if `Filter.EventType` is non-empty.
   - Append `received_at >= ?` + value if `Filter.From` is non-zero.
   - Append `received_at <= ?` + value if `Filter.To` is non-zero.
   - If conditions is non-empty, join with ` AND ` and prepend `WHERE `.
   - Append `ORDER BY received_at DESC, event_id DESC LIMIT ?` + limit value.
3. Execute with collected parameter values.
4. Scan rows into `[]Observation`.
5. Return `Page{Items: results, NextCursor: ""}`.

All values are bound as parameters (NFR-003). No SQL string interpolation occurs.

---

### Context propagation and transaction semantics

All repository methods accept `context.Context` as the first argument and pass it to
`*sql.DB` methods (`QueryContext`, `ExecContext`, `BeginTx`).

The following outcomes are possible for `AppendObservation`:

| Timing of cancellation | Outcome | Caller recovery |
|---|---|---|
| Before or during transaction, before Commit | Context error returned; no row written | Safe to retry; no data was persisted |
| Concurrent with Commit (race) | Result indeterminate from caller's perspective | Caller should call `GetObservation(event_id)` to determine state, then retry idempotently if not found |
| After Commit completes | Commit result returned (success or DB error); context cancellation does not undo a completed Commit | Row is persisted; idempotent retry returns original Receipt |

The Go `database/sql` package and the underlying SQLite driver surface context cancellation
as errors. When a cancellation error is received after beginning a transaction, the `store`
layer explicitly calls `tx.Rollback()`. If `Rollback` itself returns an error (e.g., because
the transaction was already committed), that error is silently discarded and the original
context error is returned to the caller. The library emits no internal logs; the caller is
responsible for logging at its own boundary.

> **Note on REQ-001 AC-3**: The requirement states "before the write completes", which covers
> the case where cancellation is detected before Commit. The race case above (commit/cancel
> concurrency) is not covered by AC-3 and is handled here as implementation guidance.
> No requirements.md change is required.

---

### Default application

Defaults are applied in the `store` layer before calling the repository, not inside the
repository. This keeps the repository a pure persistence layer and makes defaults testable
without a DB.

| Field | Default condition | Applied value |
|---|---|---|
| `visibility` | Empty string | `"private"` |
| `provenance.trust_level` | Empty string | `"unverified"` |

---

### Append-only enforcement

**Scope of guarantee**: Append-only semantics apply to operations performed through
Millefeuille's public `Store` API. Millefeuille does not provide cryptographic tamper
evidence and does not prevent direct modification of the SQLite database file outside the
library. Protection of the database file at the filesystem level is the responsibility of
the hosting application.

Specific guarantees provided by M0.1:
- No `Update` or `Delete` method exists on the `Store` interface or `ObservationRepository`.
- The `observations` table schema defines no `DELETE` or `UPDATE` triggers.
- The only write path is `INSERT … ON CONFLICT DO NOTHING`, which never modifies existing rows.
- A previously stored observation returned by `GetObservation` or `QueryObservations`
  reflects the exact content recorded at insertion time.

Not guaranteed in M0.1:
- Cryptographic proof of non-tampering.
- Protection against direct SQLite file access.
- Audit trail integrity against a privileged attacker with filesystem access.

Future stages may add Merkle-chain or hash-chaining mechanisms for stronger tamper evidence.

---

## Components and responsibilities

| Component | Responsibility | Related requirements |
|---|---|---|
| `internal/model` package | Domain type definitions (`Observation`, `InputObservation`, `Provenance`, `Receipt`, `Filter`, `Page`); no dependencies on other packages | REQ-005, COMP-001 |
| `Store` interface | Public API contract; defines `AppendObservation`, `GetObservation`, `QueryObservations`, `Close` | COMP-002, REQ-001–004 |
| Root package type aliases | Re-export `internal/model` types to preserve the public API surface | REQ-005, COMP-001, COMP-002 |
| `store` struct | Wires repo, clock, and config; applies defaults; validates input; validates JSON; enforces payload size | REQ-001, REQ-002, REQ-005, NFR-005, SEC-001, SEC-002 |
| `ObservationRepository` interface | Internal persistence contract; SQL operations only; uses `internal/model` types | REQ-001–004, REQ-006 |
| `sqliteRepository` | Implements `ObservationRepository` using `*sql.DB`; all SQL uses parameterized queries | NFR-001–004, SEC-003 |
| `Migrator` | Applies versioned SQL migrations; tracks applied versions in `schema_migrations` table | REQ-007 |
| `hash` package | Computes canonical SHA-256 content hash for idempotency; uses `internal/model` types | REQ-002 |
| `ClockFunc` | Injectable clock in the root package; defaults to `time.Now`; enables deterministic tests | REQ-005 AC-4 |
| `Open()` | Constructs `store`; runs `Migrator`; applies and verifies PRAGMAs; returns ready `Store` | NFR-000, REQ-007 |

---

## Data flow

### AppendObservation (new event)

```
Caller → store.AppendObservation(ctx, input)
  → validate required fields
  → json.Valid(payload)
  → check payload size (byte length)
  → apply defaults (visibility, trust_level)
  → received_at = clock().UTC()  ← assigned after validation passes; not DB commit time
  → content_hash = hash.Canonical(projection excluding received_at)  ← normalise only for hash; raw payload bytes stored as-is
  → sqliteRepository.Insert(ctx, obs, content_hash)
      → BEGIN TRANSACTION
      → INSERT INTO observations … ON CONFLICT(event_id) DO NOTHING
      → rows_affected == 1 → COMMIT
      → return Receipt{EventID, ReceivedAt, ContentHash}
  ← Receipt
```

### AppendObservation (duplicate, idempotent)

```
Caller → store.AppendObservation(ctx, input)
  → … (validate, json.Valid, defaults, hash as above)
  → sqliteRepository.Insert(ctx, obs, content_hash)
      → BEGIN TRANSACTION
      → INSERT … ON CONFLICT DO NOTHING → rows_affected == 0
      → SELECT content_hash, received_at, event_id FROM observations WHERE event_id = ?
      → stored_hash == new_hash → ROLLBACK
      → return stored Receipt
  ← original Receipt (with original received_at)
```

### AppendObservation (conflict)

```
  → … rows_affected == 0
  → stored_hash != new_hash → ROLLBACK
  ← ErrConflict
```

### GetObservation

```
Caller → store.GetObservation(ctx, eventID)
  → sqliteRepository.FindByEventID(ctx, eventID)
      → SELECT … WHERE event_id = ?
      → row found → map → Observation
      → row not found → ErrNotFound
  ← Observation or ErrNotFound
```

### QueryObservations

```
Caller → store.QueryObservations(ctx, filter)
  → normalise limit (0→100, >1000→1000)
  → sqliteRepository.Query(ctx, filter)
      → build parameterized SQL
      → SELECT … [WHERE …] ORDER BY received_at DESC, event_id DESC LIMIT ?
      → scan rows
  ← Page{Items: []Observation, NextCursor: ""}
```

---

## API changes

This is a new library. The public Go API is defined here for the first time.

### Types (package `millefeuille`)

> **Package note**: The domain types below are defined in `internal/model` and
> re-exported from the root `millefeuille` package via type aliases
> (e.g., `type Observation = model.Observation`). This keeps the public API
> identical while allowing `internal/sqlite` and `internal/hash` to reference the
> types without importing the root package (avoiding an import cycle). The field
> definitions and public contract are unchanged.

```go
// InputObservation is the caller-supplied input to AppendObservation.
// ReceivedAt is absent; the library assigns it.
type InputObservation struct {
    EventID       string
    SchemaVersion string
    SourceID      string
    DeviceID      string          // optional; empty string if absent
    EventType     string
    ObservedAt    time.Time
    Payload       json.RawMessage // must be valid JSON; max 1 MiB
    Visibility    string          // optional; defaults to "private"
    Provenance    Provenance
}

// Observation is the stored and returned form; includes library-assigned fields.
type Observation struct {
    EventID       string
    SchemaVersion string
    SourceID      string
    DeviceID      string
    EventType     string
    ObservedAt    time.Time
    ReceivedAt    time.Time       // library-assigned UTC timestamp
    Payload       json.RawMessage
    Visibility    string
    Provenance    Provenance
}

type Provenance struct {
    Origin     string  // "device" | "agent" | "simulator" | "external"
    TrustLevel string  // default "unverified"
}

type Receipt struct {
    EventID     string
    ReceivedAt  time.Time
    ContentHash string  // hex-encoded SHA-256 of canonical content projection (excludes received_at)
}

type Filter struct {
    SourceID  string
    EventType string
    From      time.Time  // inclusive lower bound on received_at; zero = no bound
    To        time.Time  // inclusive upper bound on received_at; zero = no bound
    Limit     int        // 0 = default 100; > 1000 = capped to 1000; negative = ErrInvalidFilter
}

type Page struct {
    Items      []Observation
    NextCursor string  // always "" in M0.1; reserved for M0.2 cursor pagination
}

// ClockFunc is the injectable time source.
type ClockFunc func() time.Time
```

### Store interface

```go
type Store interface {
    AppendObservation(ctx context.Context, obs InputObservation) (Receipt, error)
    GetObservation(ctx context.Context, eventID string) (*Observation, error)
    QueryObservations(ctx context.Context, filter Filter) (Page, error)
    Close() error
}

// Open constructs a Store backed by a file-based SQLite database at the given path.
// It runs pending migrations, applies connection PRAGMAs, and verifies WAL mode
// before returning. In-memory SQLite paths (":memory:", "file::memory:") are not
// supported and will return an error.
func Open(ctx context.Context, path string, opts ...Option) (Store, error)
```

### Sentinel errors

```go
var (
    ErrNotFound           = errors.New("observation not found")
    ErrConflict           = errors.New("conflicting observation for event_id")
    ErrInvalidObservation = errors.New("invalid observation")
    ErrPayloadTooLarge    = errors.New("payload exceeds 1 MiB limit")
    ErrInvalidFilter      = errors.New("invalid query filter")
)
```

---

## Data-model changes

### SQLite schema (migration 001)

```sql
CREATE TABLE IF NOT EXISTS observations (
    event_id        TEXT        NOT NULL,
    schema_version  TEXT        NOT NULL,
    source_id       TEXT        NOT NULL,
    device_id       TEXT        NOT NULL DEFAULT '',
    event_type      TEXT        NOT NULL,
    observed_at     TEXT        NOT NULL,  -- nanosecond UTC: 2006-01-02T15:04:05.000000000Z
    received_at     TEXT        NOT NULL,  -- nanosecond UTC: 2006-01-02T15:04:05.000000000Z; library-assigned
    payload         TEXT        NOT NULL,  -- raw input JSON bytes stored as-is; validity enforced in Go; max 1 MiB; NOT re-normalised before storage
    visibility      TEXT        NOT NULL DEFAULT 'private',
    provenance      TEXT        NOT NULL,  -- JSON: {"origin":"...","trust_level":"..."}
    content_hash    TEXT        NOT NULL,  -- hex SHA-256 of canonical projection (excludes received_at)
    PRIMARY KEY (event_id)
);

CREATE INDEX IF NOT EXISTS idx_observations_source_id
    ON observations (source_id);

CREATE INDEX IF NOT EXISTS idx_observations_event_type
    ON observations (event_type);

CREATE INDEX IF NOT EXISTS idx_observations_received_at
    ON observations (received_at DESC);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER NOT NULL PRIMARY KEY,
    applied_at  TEXT    NOT NULL  -- nanosecond UTC
);
```

**Index rationale:**
- `source_id`, `event_type`, and `received_at` are the filter/sort columns named in REQ-004.
- `device_id` is intentionally not indexed (OQ-001 / M0.2 deferral).
- `event_id` is the PRIMARY KEY; no separate index needed.
- A composite index on `(received_at DESC, event_id DESC)` would better serve the sort
  order; a single-column index on `received_at` is chosen for M0.1 simplicity and can be
  replaced with a composite index in M0.2 if profiling warrants it.

**Timestamp storage format:**
All timestamp columns use a fixed nanosecond-precision UTC string:
`2006-01-02T15:04:05.000000000Z` (Go format: `"2006-01-02T15:04:05.000000000Z"`).

Rationale:
- Nanosecond precision preserves the full resolution of `time.Time` without loss.
- The fixed-width format ensures that SQLite's lexicographic string comparison produces
  correct chronological ordering for `ORDER BY received_at DESC`.
- UTC normalisation guarantees timezone-consistent storage (NFR-004).

Note: the OS clock may not provide nanosecond-level accuracy in practice. The format
guarantees no precision is lost in serialisation; it does not guarantee that the OS clock
is accurate to the nanosecond.

Go marshal: `t.UTC().Format("2006-01-02T15:04:05.000000000Z")`
Go unmarshal: `time.Parse("2006-01-02T15:04:05.000000000Z", s)` → call `.UTC()` on result.

**String field lengths:** Requirements (REQ-005 AC-6) mandate that maximum lengths be
decided in detailed design before implementation. The following limits are adopted for M0.1
and enforced in the `store` validation layer (not at the SQL schema level):

| Field | Max bytes | Rationale |
|---|---|---|
| `event_id` | 128 | Accommodates UUIDs (36), ULIDs (26), and reasonable custom formats |
| `schema_version` | 32 | Short free-form version string |
| `source_id` | 256 | Device IDs, agent names, URN-style identifiers |
| `device_id` | 256 | Same rationale as `source_id`; empty string is valid |
| `event_type` | 128 | Dot-separated hierarchy (e.g., `device.sensor.temperature`) |
| `visibility` | 64 | Enumerated in practice (`private`, `public`, etc.) |
| `provenance.origin` | 64 | Enumerated values (`device`, `agent`, `simulator`, `external`) |
| `provenance.trust_level` | 64 | Enumerated values (`unverified`, etc.) |

Violations return `ErrInvalidObservation` with the field name and limit. SQL `CHECK`
constraints may be added in M0.2 if enforcement at the DB layer is desired.

### Migration table

`schema_migrations` tracks applied migration versions. `Migrator` queries the table at
startup, compares against the compiled list of migrations, and executes any pending ones in
ascending version order within individual transactions. The Migrator also validates at
startup that the compiled migration list is contiguous and sorted (no version gaps); it
returns an error if not.

---

## SQLite connection management

### Selected driver

**Driver**: `modernc.org/sqlite` (pure Go, CGO-free).

Rationale:
- No CGO dependency; simplifies cross-compilation and CI setup (consistent with the
  Go standard library preference in Constraints).
- Actively maintained; supports WAL mode, all required PRAGMAs, and standard
  `database/sql` interface.
- Widely used in production Go projects.

### File-based database only

M0.1 supports file-based SQLite databases only. Paths matching `:memory:` or containing
`mode=memory` in a URI are rejected by `Open()` with an error. This simplifies WAL
enforcement and ensures data persistence across process restarts (REQ-001 AC-2).

Tests use file-based databases in `t.TempDir()`.

### Per-connection PRAGMA initialisation

`modernc.org/sqlite` supports PRAGMA application via DSN query parameters using the
`_pragma` key. Parameters specified in the DSN are applied by the driver on every new
connection it opens, including connections created to replace a dropped one. This is the
standard approach for `modernc.org/sqlite` and requires no driver-internal hooks or
custom connector wrappers.

DSN format:

```
file:/path/to/db.sqlite?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)
```

The `journal_mode` PRAGMA is included in the DSN as a belt-and-suspenders measure. Because
WAL mode is persistent in the database file, this is mainly relevant on the first connection
to a fresh file. On subsequent opens it is a no-op. The `Open()` function still verifies
WAL mode explicitly (see Connection initialisation sequence) to fail fast if the DB file
was inadvertently reset.

`synchronous`, `busy_timeout`, and `foreign_keys` are connection-level settings and are
re-applied on every new connection via the DSN.

### PRAGMA settings

The following PRAGMAs are applied to **every connection** when it is acquired from the pool:

| PRAGMA | Value | Rationale |
|---|---|---|
| `journal_mode` | `WAL` | Required by NFR-000; verified after setting (persists on disk once set) |
| `synchronous` | `FULL` | Crash-safe committed writes; relies on OS/FS honouring fsync |
| `busy_timeout` | `5000` (ms) | Prevents immediate `SQLITE_BUSY` under light read concurrency |
| `foreign_keys` | `ON` | Defensive hygiene; no FK in M0.1 schema but prevents future mistakes |

Note: `journal_mode=WAL` is a persistent database-level setting in SQLite; once set it
survives close/reopen. However, `synchronous`, `busy_timeout`, and `foreign_keys` are
connection-level settings and **must** be re-applied on every new connection.

### Connection initialisation sequence

`Open()` performs the following steps in order:

1. Build the DSN string with the `_pragma` query parameters for all required PRAGMAs.
2. Open `*sql.DB` via `sql.Open("sqlite", dsn)`.
3. Call `db.PingContext(ctx)` to force-open at least one connection and confirm the DB
   file is accessible and PRAGMAs are applied.
4. Verify WAL mode: execute `PRAGMA journal_mode` on an acquired connection and confirm
   the returned value is `"wal"`; return error if not.
5. Run `Migrator.Migrate(ctx)`.
6. Return the constructed `store`.

### Connection pool settings

M0.1 targets a single application process with one logical writer and concurrent readers
within that process. `modernc.org/sqlite` supports WAL-mode concurrent reads within a
single process using multiple connections.

| Setting | Value | Rationale |
|---|---|---|
| `SetMaxOpenConns` | 1 | M0.1 design choice: a single connection slot serialises all DB operations within the process, eliminating `SQLITE_BUSY` errors between goroutines. This trades WAL-mode parallel read throughput for simpler conflict-free operation. Acceptable for M0.1's single-process moderate-load use case. |
| `SetMaxIdleConns` | 1 | Matches `MaxOpenConns`; retains the single connection in the idle pool |
| `SetConnMaxLifetime` | 0 (no limit) | Long-lived connection; no rotation needed in M0.1 |
| `SetConnMaxIdleTime` | 0 (no limit) | Consistent with single-process, long-running usage |

> **Rationale for `SetMaxOpenConns(1)`**: This is an M0.1 design choice to prioritise
> simplicity and conflict-free operation, not a hard constraint of `modernc.org/sqlite`.
> A single connection slot eliminates `SQLITE_BUSY` errors between goroutines sharing the
> same process and makes transaction ordering straightforward. The trade-off is that
> concurrent reads do not benefit from WAL-mode parallelism; goroutines wait for the
> single slot. For M0.1's expected load this is acceptable. If higher read concurrency
> is needed in M0.2+, a read/write pool split can be introduced without API changes.

### WAL checkpoint

SQLite performs automatic checkpoints when the WAL file exceeds the default threshold
(1000 pages). No explicit checkpoint scheduling is implemented in M0.1.

`Close()` closes the `*sql.DB` connection cleanly (calling `db.Close()`). A forced
`wal_checkpoint(FULL)` is **not** required on close. SQLite will correctly re-open the
database including any un-checkpointed WAL pages on the next `Open()`. This is standard
SQLite behaviour and does not indicate data loss.

---

## Error handling

| Condition | Handling | Related requirements |
|---|---|---|
| Required field missing or empty | Return `ErrInvalidObservation` with field name in message; no DB write | REQ-001 AC-5 |
| Required string field exceeds maximum length | Return `ErrInvalidObservation` with field name and limit; no DB write | REQ-005 AC-6 |
| `Filter.Limit` is negative | Return `ErrInvalidFilter`; no DB query | REQ-004 |
| `payload` is not valid JSON | Return `ErrInvalidObservation` with description; no DB write | REQ-001 AC-5 |
| Payload > 1 MiB (byte length) | Return `ErrPayloadTooLarge`; no DB write | REQ-001 AC-4, NFR-005 |
| `event_id` duplicate, same hash | Return original `Receipt`; no insert | REQ-002 AC-1 |
| `event_id` duplicate, different hash | Return `ErrConflict`; stored row unchanged | REQ-002 AC-2 |
| `event_id` not found in `GetObservation` | Return `ErrNotFound` (sentinel, not wrapped) | REQ-003 AC-2 |
| Context cancelled before Commit | Surface `ctx.Err()`; rollback explicitly; no row written | REQ-001 AC-3, NFR-002 |
| Context cancelled concurrent with Commit | Result indeterminate; return error; caller recovers via `GetObservation` + idempotent retry | NFR-002 (implementation guidance) |
| Context cancelled after Commit | Commit result returned; cancellation does not undo committed data | NFR-002 |
| Context cancelled in `GetObservation` / `QueryObservations` | Surface `ctx.Err()` from driver | REQ-003 AC-3, REQ-004 AC-10 |
| WAL mode cannot be verified on `Open` | Return error; `Store` not constructed | NFR-000 |
| In-memory DB path passed to `Open` | Return error; `Store` not constructed | NFR-000 |
| Migration failure | Rollback failed migration transaction; return error; do not proceed | REQ-007 AC-3 |
| SQLite busy / locked | `busy_timeout=5000ms` applied; surface `database/sql` error if timeout exceeded | NFR-000, NFR-001 |
| `tx.Rollback()` error after context cancel | Discard the rollback error; return the original context error to caller; no internal logging | NFR-002 |

Errors are not wrapped with stack traces in M0.1. Callers use `errors.Is` to inspect
sentinels. Contextual messages are conveyed via
`fmt.Errorf("missing field %q: %w", fieldName, ErrInvalidObservation)`.

---

## Security considerations

- **Parameterized queries**: Every SQL statement uses `?` placeholders. No user-supplied
  value is ever concatenated into SQL strings (NFR-003, SEC-003). Dynamic `WHERE` clauses
  in `QueryObservations` are constructed by appending fixed clause fragments and collecting
  values in a separate `[]any` slice.
- **Payload opaqueness**: Millefeuille stores `payload` as the raw input JSON bytes without
  interpreting its semantic meaning. `json.Valid` is called to reject malformed JSON before
  storage. For `content_hash` computation only, the payload is decoded with `UseNumber()`
  and re-encoded via `json.Marshal` to produce a key-order-normalised canonical form; this
  decode/encode cycle is performed in memory and does not alter the bytes written to the
  database.
- **Visibility label preservation**: The `visibility` field is persisted and returned
  verbatim. Millefeuille does not filter query results based on visibility; the caller
  (IMAGO Agent / Guardian) must enforce access control at its boundary (SEC-001).
- **Trust level**: `trust_level` is stored as provided by the caller. Millefeuille never
  upgrades a trust level automatically (SEC-002).
- **No network surface**: The library has no listener, server, or outbound connection.
  Attack surface is limited to the SQLite file and the Go API.
- **SQLite file permissions**: Out of scope for M0.1; the hosting application is responsible
  for setting appropriate filesystem permissions on the database file and associated WAL
  (`-wal`) and shared-memory (`-shm`) files.

**Security notice for integrators** (to be reproduced in README / API documentation):
- Millefeuille is designed to be called from a trusted application layer. It performs no
  authentication or authorisation of its own.
- `visibility: "private"` is a metadata label. Millefeuille does not enforce access control
  based on this field; the hosting application must do so.
- Do not expose Millefeuille's `Store` interface to untrusted callers without an
  authorisation layer (e.g., Guardian) in between.
- The SQLite database file, `-wal` file, and `-shm` file must be protected with appropriate
  filesystem permissions by the hosting application.

---

## Compatibility and migration

- The `Store` interface is the stable public contract. Internal types (`sqliteRepository`,
  `Migrator`) are unexported and may change without notice.
- `Page.NextCursor` is always `""` in M0.1. M0.2 will populate this field; existing callers
  that ignore it will continue to work without modification (COMP-001).
- `InputObservation` has no `ReceivedAt` field. If a future version needs to accept an
  externally provided timestamp, a new input type or option will be introduced rather than
  adding `ReceivedAt` to `InputObservation`, preserving the current contract.
- Adding new optional filter fields to `Filter` in M0.2 is backward-compatible (zero value
  = no filter).
- Schema migrations are additive only. No migration drops a column or changes a column type.
  Migration version numbers are monotonically increasing integers.
- `Receipt.ContentHash` (renamed from the initial `PayloadHash` draft) is the canonical
  name for the SHA-256 digest field. All design documents, tests, and code use `ContentHash`.
  See requirements.md REQ-001 AC-1 for the corresponding update.

---

## Observability

- **Logs**: M0.1 does not emit structured logs from within the library. The caller is
  expected to log at its own boundary. If logging is added in M0.2, it will be via an
  injectable `slog.Logger` option rather than a global logger.
- **Metrics**: Not in scope for M0.1.
- **Traces**: Not in scope for M0.1.
- **Errors**: All errors are returned to the caller; none are silently swallowed.
  `tx.Rollback()` errors after context cancellation are the only exception: they are
  expected and discarded (not returned) to avoid masking the original context error.
  No internal logging is performed; the library surfaces errors upward only.

---

## Test strategy

| Requirement | Test level | Verification |
|---|---|---|
| REQ-001 AC-1 | Unit | `AppendObservation` returns `Receipt` with `event_id`, `received_at`, `content_hash` |
| REQ-001 AC-2 | Integration | Observation survives `store.Close()` + `Open()` on file-based DB and is retrievable |
| REQ-001 AC-3 | Unit | Cancelled context (before Commit) returns `ctx.Err()`; `GetObservation` confirms no row written |
| REQ-001 AC-4 | Unit | Payload of 1 MiB + 1 byte returns `ErrPayloadTooLarge` |
| REQ-001 AC-5 | Unit | Each required field omitted individually returns `ErrInvalidObservation`; invalid JSON payload returns `ErrInvalidObservation` |
| REQ-002 AC-1 | Unit | Re-submit identical observation → same `Receipt`, one DB row |
| REQ-002 AC-2 | Unit | Re-submit with same `event_id` but different payload → `ErrConflict` |
| REQ-002 AC-3/4 | Unit | Re-submit returns original `received_at` (injected clock advances between calls) |
| REQ-003 AC-1 | Unit | `GetObservation` returns stored `Observation` |
| REQ-003 AC-2 | Unit | `GetObservation` with unknown ID returns `ErrNotFound` |
| REQ-003 AC-3 | Unit | Cancelled context returns `ctx.Err()` |
| REQ-004 AC-1 | Unit | Results ordered `received_at DESC, event_id DESC` |
| REQ-004 AC-2 | Unit | Limit 0 → at most 100 rows returned |
| REQ-004 AC-3 | Unit | Limit 9999 → at most 1000 rows returned |
| REQ-004 AC-4–7 | Unit | Filter combinations: `source_id`, `event_type`, time range, AND composition |
| REQ-004 AC-8 | Unit | No matching rows → empty `Page` |
| REQ-004 AC-9 | Unit | `Page.NextCursor` is always `""` |
| REQ-005 | Compile-time + Unit | `InputObservation` has no `ReceivedAt` field; returned `Observation` has it set |
| REQ-005 AC-3 | Unit | Injected clock returns fixed time; `received_at` in `Receipt` matches exactly |
| REQ-006 | Compile-time | `Store` interface has no `Update` or `Delete` method |
| REQ-007 AC-1 | Integration | Fresh file-based DB: migrations applied; schema at current version |
| REQ-007 AC-2 | Integration | Re-open DB: migrations skipped (idempotent) |
| REQ-007 AC-3 | Integration | Failing migration rolls back; schema unchanged |
| NFR-000 | Integration | `Open` on file-based DB → `PRAGMA journal_mode` returns `wal` |
| NFR-000 | Unit | `Open` with in-memory path (`:memory:`) returns error |
| NFR-001 | Integration | Context-cancel during transaction → no partial row; confirmed via `GetObservation` |
| NFR-004 | Unit | Timestamps stored and returned as UTC nanosecond-precision strings; timezone-offset input normalised to UTC |
| SEC-001 | Unit | `visibility` defaults to `"private"` when not provided |
| SEC-002 | Unit | `trust_level` defaults to `"unverified"` when not provided |
| Hash canonicalisation | Unit | Byte-identical inputs → same hash |
| Hash canonicalisation | Unit | `payload` differing only in whitespace at any level → same hash |
| Hash canonicalisation | Unit | `payload` differing only in key order at any nesting level → same hash (normalised by `json.Marshal` map-key sort) |
| Hash canonicalisation | Unit | `payload` differing in numeric representation (`1` vs `1.0`) → different hash (documented) |
| Hash canonicalisation | Unit | `payload` with duplicate keys → same hash after last-key-wins resolution; idempotency holds between two such submissions |
| PRAGMA verification | Integration | After `Open()`: query each of `journal_mode`, `synchronous`, `busy_timeout`, `foreign_keys` and assert expected values (`wal`, `2` for FULL, `5000`, `1`) |
| Connection leak | Integration | Simulate `Open()` failure after DB open but before migration completes; verify `db.Close()` is called and no file handle leaks |
| Negative limit | Unit | `QueryObservations` with `Filter.Limit = -1` returns `ErrInvalidFilter` |

**Test tooling**: Go standard `testing` package. No external test framework. All integration
tests use file-based SQLite in `t.TempDir()`. The injectable `ClockFunc` eliminates the
need for time-freezing libraries.

---

## Alternatives considered

### Alternative 1: Expose `received_at` as a caller-supplied field

- Description: Accept `received_at` from the caller (as in the original Assumptions draft),
  allowing the gateway to supply its own receipt timestamp.
- Rejected because: It moves the source of truth outside the library, making the timestamp
  unverifiable and requiring the caller to handle time synchronization. Assigning it
  internally (DD-002) gives a consistent, trusted acceptance time and simplifies the caller
  contract.

### Alternative 2: Use an ORM (e.g., GORM, ent)

- Description: Use an ORM to manage schema, migrations, and queries.
- Rejected because: ORMs add significant transitive dependencies and abstract away the SQL
  that must be carefully controlled for parameterization (SEC-003) and append-only semantics
  (REQ-006). Plain `database/sql` with explicit SQL is auditable and has zero extra
  dependencies.

### Alternative 3: Store `provenance` as individual columns

- Description: Expand `provenance.origin` and `provenance.trust_level` into separate
  `TEXT` columns instead of a JSON blob.
- Rejected because: The `provenance` object is defined as extensible (future fields may be
  added). Storing it as JSON preserves forward compatibility without schema migrations for
  every new provenance field. The trade-off is that individual provenance fields are not
  directly filterable in SQL, which is acceptable since REQ-004 does not require filtering
  by provenance in M0.1.

### Alternative 4: Use a separate `receipts` table

- Description: Store `Receipt` data (`content_hash`, `received_at`) in a dedicated table
  joined to `observations`.
- Rejected because: In M0.1 there is a 1:1 relationship between an observation and its
  receipt. A separate table adds join complexity with no benefit. If receipt history (e.g.,
  multiple re-append attempts) becomes a requirement in a future stage, the schema can be
  extended then.

### Alternative 5: Use RFC 3339 (second precision) for timestamp storage

- Description: Store timestamps as `time.RFC3339` strings (second precision).
- Rejected because: RFC 3339 second precision silently truncates sub-second data from
  `time.Time`. The nanosecond-precision format is chosen to preserve full `time.Time`
  resolution and to ensure correct lexicographic ordering in SQLite. The format is a strict
  superset of RFC 3339.

### Alternative 6: Force `SetMaxOpenConns(1)`

- Description: Restrict the `*sql.DB` pool to a single connection to avoid write
  concurrency issues.
- Adopted for M0.1 as a design choice that prioritises simplicity and conflict-free
  operation over concurrent read throughput. This is not a constraint specific to
  `modernc.org/sqlite`; WAL mode does allow multi-connection concurrency in principle.
  The trade-off (no parallel reads within the process) is acceptable for M0.1's
  single-process moderate load. In M0.2+ a read/write pool split can be introduced if
  higher read throughput is needed, without changing the public API.

### Alternative 7: Force `wal_checkpoint(FULL)` on `Close()`

- Description: Explicitly flush WAL to the main database file on every `Close()`.
- Rejected because: SQLite handles WAL re-opening transparently on the next `Open()`. A
  forced checkpoint on every close adds latency without improving correctness or durability.
  Automatic checkpointing at the default threshold is sufficient for M0.1.

---

## Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| `SetMaxOpenConns(1)` serialises all reads and writes | No WAL-mode parallel reads within the process; goroutines queue for the single connection slot | Accepted M0.1 design trade-off. If higher read concurrency is needed, introduce read/write pool split in M0.2+ without API changes. |
| Clock skew between `observed_at` and `received_at` | Queries by time range may return unexpected results | Document that `received_at` is the trusted sort/filter key; `observed_at` is untrusted source-reported time |
| Idempotency hash collision (SHA-256) | Two distinct observations treated as duplicates | SHA-256 collision probability is negligible; no mitigation required in M0.1 |
| Numeric representation differences in `payload` (e.g., `1` vs `1.0`) cause spurious ErrConflict | Caller retries with re-serialised numeric values treated as conflicts | Document that callers must submit numerically consistent payloads; key-order and whitespace differences are fully normalised. |
| Large payload degrades query performance | Slow `QueryObservations` when many 1 MiB payloads present | 1 MiB limit per observation; index on `received_at` limits row scans; profiling deferred to M0.2 |
| `busy_timeout` not applied to every connection | `SQLITE_BUSY` error surfaces to caller unexpectedly | Apply PRAGMAs via DSN `_pragma` query parameters; verify each PRAGMA value in integration test after `Open()` |
| Migration version gap (developer skips a number) | Incorrect version ordering | Migrator validates that version list is contiguous and sorted at startup; returns error if not |
| Context cancel concurrent with Commit — indeterminate result | Caller cannot tell if write succeeded | Document recovery path: caller calls `GetObservation(event_id)`; if found, original Receipt is returned on idempotent retry |

---

## Open questions

### DD-001 — Resolved (incorporated above)

`visibility` and `provenance` are included in the idempotency hash projection. Rationale:
both are caller-assessed fields that reflect intent. A retry that changes visibility or
trust level represents a different submission, not a transparent retry.

### DD-003 — Resolved (incorporated above)

PRAGMA settings, connection pool policy, and driver selection (`modernc.org/sqlite`) are
defined in the "SQLite connection management" section. The per-connection PRAGMA mechanism
uses DSN `_pragma` query parameters supported natively by `modernc.org/sqlite`. No open
questions remain.

---

## Applied requirements.md updates

The following changes to `requirements.md` were approved by the maintainer and applied:

| Location | Change | Approved |
|---|---|---|
| REQ-001 AC-1 | `payload_hash` → `content_hash` | 2026-10-10 |
| REQ-003 AC-2 | NotFound sentinel clarified as `ErrNotFound` returned as `error`; callers use `errors.Is` | 2026-10-10 |

> **Approval gate:** Design approved by habuka036 on 2026-10-10. Proceeding to tasks phase.
