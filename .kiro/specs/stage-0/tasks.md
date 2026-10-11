# Tasks: Millefeuille Stage 0 (M0.1) — Core Observation Storage

## Metadata

- Status: Completed
- Related spec: `.kiro/specs/stage-0/requirements.md`, `.kiro/specs/stage-0/design.md`
- Created: 2026-10-10
- Updated: 2026-10-10 (revision 4: internal/model package introduced to resolve import cycle; TASK-001/002/003/005 file lists and signatures updated)

---

## Execution order and dependencies

```
TASK-001 (module scaffold)
  └─ TASK-002 (types & errors)
       ├─ TASK-003 (hash package)
       ├─ TASK-004 (SQLite schema & migrator)
       │    └─ TASK-005 (repository)
       │         └─ TASK-006 (store + Open)
       │              ├─ TASK-007 (AppendObservation)
       │              │    └─ TASK-009 (AppendObservation tests)
       │              ├─ TASK-008 (GetObservation + QueryObservations)
       │              │    └─ TASK-010 (GetObservation + QueryObservations tests)
       │              └─ TASK-011 (integration & hash tests)
       └─ TASK-012 (README & security notice)
```

---

## Tasks

- [x] TASK-001: Initialise Go module and project skeleton
  - Requirements: REQ-001–007, COMP-002 (foundational)
  - Design: Package layout
  - Depends on: None
  - Files:
    - `go.mod`
    - `go.sum`
    - `store.go` (empty package declaration)
    - `observation.go` (empty package declaration)
    - `errors.go` (empty package declaration)
    - `clock.go` (empty package declaration)
    - `internal/model/model.go` (empty)
    - `internal/sqlite/repository.go` (empty)
    - `internal/sqlite/migrator.go` (empty)
    - `internal/sqlite/migrations/` (empty directory)
    - `internal/hash/hash.go` (empty)
  - Completion criteria:
    - `go.mod` declares module `github.com/habuka036/millefeuille` with Go version
    - `modernc.org/sqlite` is the only non-stdlib dependency
    - `go build ./...` succeeds with no errors
  - Verification: `go build ./...`

---

- [x] TASK-002: Define domain types (internal/model) and sentinel errors
  - Requirements: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, COMP-001, COMP-002
  - Design: API changes — Types; Sentinel errors; Package layout (internal/model)
  - Depends on: TASK-001
  - Files:
    - `internal/model/model.go`
    - `observation.go` (type aliases re-exporting model types)
    - `errors.go`
    - `clock.go`
    - `store.go` (Store interface)
  - Completion criteria:
    - `internal/model` package defines `InputObservation`, `Observation`, `Provenance`, `Receipt`, `Filter`, `Page`; it imports no other package in this module
    - `InputObservation` has no `ReceivedAt` field
    - `Observation` includes `ReceivedAt time.Time`
    - `Provenance` has `Origin` and `TrustLevel`
    - `Receipt` has `EventID`, `ReceivedAt`, `ContentHash`
    - `Filter` defined; `Limit` comment notes `negative = ErrInvalidFilter`
    - `Page` has `Items []Observation` and `NextCursor string`
    - Root `observation.go` re-exports every model type via type alias (`type Observation = model.Observation`, etc.) so the public API is unchanged
    - `ClockFunc` type defined as `func() time.Time` in the root package (`clock.go`)
    - All five sentinel errors defined in root `errors.go`: `ErrNotFound`, `ErrConflict`, `ErrInvalidObservation`, `ErrPayloadTooLarge`, `ErrInvalidFilter`
    - `Store` interface defined with `AppendObservation`, `GetObservation`, `QueryObservations`, `Close`
    - `go build ./...` succeeds with no import cycle
    - Absence of `ReceivedAt` in `InputObservation` is verified by a runtime test using `reflect.TypeOf(InputObservation{}).FieldByName("ReceivedAt")`; the test asserts the field is not found (`ok == false`); if `ReceivedAt` is accidentally added the test fails
  - Verification: `go build ./...`; `go test ./...` (reflect runtime test for absent field); compile-time check that `Store` has no `Update` or `Delete` method (REQ-006)

---

- [x] TASK-003: Implement canonical content hash
  - Requirements: REQ-002
  - Design: Idempotency and content hash; hash package
  - Depends on: TASK-002
  - Files:
    - `internal/hash/hash.go`
    - `internal/hash/hash_test.go`
  - Completion criteria:
    - `Canonical(obs model.InputObservation) (string, error)` computes SHA-256 hex digest (uses `internal/model` types; does not import the root package)
    - Projection includes: `event_id`, `schema_version`, `source_id`, `device_id`, `event_type`, `observed_at`, `payload`, `visibility`, `provenance.origin`, `provenance.trust_level`
    - `received_at` is excluded from the projection
    - `observed_at` serialised as `2006-01-02T15:04:05.000000000Z` UTC
    - `device_id` included as `""` when empty
    - `payload` normalised via `json.NewDecoder` + `UseNumber()` decode then `json.Marshal` re-encode
    - After decoding `payload`, the decoder is checked to confirm the entire input was consumed (e.g., a second `Decode` call must return `io.EOF`); if trailing data is present `Canonical` returns an error
    - Tests pass for all canonicalisation cases below
  - Verification: `go test ./internal/hash/...`
  - Test cases (failing tests written first):
    - Byte-identical inputs → same hash
    - `payload` differing only in whitespace at any level → same hash
    - `payload` differing only in key order at any nesting level → same hash
    - `payload` differing in numeric representation (`1` vs `1.0`) → different hash (documented)
    - `payload` with duplicate keys → deterministic hash after last-key-wins resolution; idempotency holds
    - `payload` with trailing data after valid JSON (e.g., `{"k":1}garbage`) → `Canonical` returns error

---

- [x] TASK-004: Implement SQLite schema migration
  - Requirements: REQ-007, NFR-000
  - Design: Data-model changes — SQLite schema; Migration table; Components — Migrator
  - Depends on: TASK-002
  - Files:
    - `internal/sqlite/migrations/001_initial_schema.sql`
    - `internal/sqlite/migrator.go`
    - `internal/sqlite/migrator_test.go`
  - Completion criteria:
    - `001_initial_schema.sql` creates `observations` table and `schema_migrations` table with all columns and indexes as specified in design
    - `observations` PRIMARY KEY is `event_id`
    - Indexes on `source_id`, `event_type`, `received_at DESC`
    - No index on `device_id`
    - `Migrator.Migrate(ctx)` applies pending migrations in ascending version order within individual transactions
    - Already-applied migrations are skipped
    - Failed migration is rolled back; schema left unchanged
    - Migrator validates that compiled migration list is contiguous and sorted; returns error if not
    - Tests use file-based SQLite in `t.TempDir()`
  - Verification: `go test ./internal/sqlite/...`
  - Test cases:
    - Fresh DB: migrations applied; `schema_migrations` contains version 1
    - Re-open DB: `Migrate` called again; migrations skipped (idempotent)
    - Failing migration (injected): transaction rolled back; schema unchanged

---

- [x] TASK-005: Implement SQLite repository
  - Requirements: REQ-001, REQ-002, REQ-003, REQ-004, REQ-006, NFR-001, NFR-003, NFR-004
  - Design: Components — sqliteRepository; Data flow; Error handling
  - Depends on: TASK-004
  - Files:
    - `internal/sqlite/repository.go`
    - `internal/sqlite/repository_test.go`
  - Completion criteria:
    - `ObservationRepository` interface defined with `Insert(ctx context.Context, obs model.Observation, contentHash string) (model.Receipt, error)`, `FindByEventID(ctx context.Context, eventID string) (*model.Observation, error)`, `Query(ctx context.Context, filter model.Filter) ([]model.Observation, error)` (uses `internal/model` types; does not import the root package)
    - `Insert` owns the full transaction lifecycle and all idempotency/conflict logic:
      1. `BeginTx`
      2. `INSERT INTO observations … ON CONFLICT(event_id) DO NOTHING`
      3. If rows-affected == 1: `Commit`; return the `Receipt` for the newly inserted row
      4. If rows-affected == 0: `SELECT content_hash, received_at FROM observations WHERE event_id = ?`
         - Stored hash == incoming `contentHash`: `Rollback`; return the stored `Receipt` (idempotent success, `nil` error)
         - Stored hash != incoming `contentHash`: `Rollback`; return `(Receipt{}, ErrConflict)`
    - Context cancellation inside `Insert` triggers explicit `Rollback`; `Rollback` error is silently discarded; the originating context error is returned
    - `store.AppendObservation` calls `repository.Insert` and propagates the returned `(Receipt, error)` directly to its caller; it does not perform hash comparison or own any SQLite transaction
    - All SQL uses parameterized queries; no string interpolation
    - Timestamps stored and read as `2006-01-02T15:04:05.000000000Z` UTC format
    - `FindByEventID` returns `*Observation` or `ErrNotFound`; passes ctx to driver
    - `Query` builds dynamic WHERE clause with AND; normalises limit (0→100, >1000→1000); rejects negative limit with `ErrInvalidFilter`; orders by `received_at DESC, event_id DESC`
    - Context cancellation surfaces as `ctx.Err()` from `database/sql`
  - Verification: `go test ./internal/sqlite/...`

---

- [x] TASK-006: Implement Store, Open, and connection management
  - Requirements: REQ-007, NFR-000, NFR-002, COMP-002
  - Design: SQLite connection management; Components — Open(), store struct; Proposed design — Overview
  - Depends on: TASK-005
  - Files:
    - `store.go`
  - Completion criteria:
    - `Open(ctx, path, opts)` constructs DSN with `_pragma` parameters for `journal_mode(WAL)`, `synchronous(FULL)`, `busy_timeout(5000)`, `foreign_keys(ON)`
    - `Open` rejects `:memory:` and `file::memory:` paths with error
    - `Open` calls `db.PingContext(ctx)`, verifies `PRAGMA journal_mode` returns `"wal"`, runs `Migrator.Migrate(ctx)`; on any failure calls `db.Close()` before returning error (no connection leak)
    - `SetMaxOpenConns(1)`, `SetMaxIdleConns(1)`, `SetConnMaxLifetime(0)`, `SetConnMaxIdleTime(0)` applied
    - `store` struct holds `*sql.DB`, `ObservationRepository`, `ClockFunc`
    - `store.Close()` calls `db.Close()` only; no forced checkpoint
    - `Option` functional-options pattern defined for `ClockFunc` injection
  - Verification: `go test ./...`
  - Test cases:
    - `Open` with in-memory path returns error
    - After `Open`: query each of `journal_mode`, `synchronous`, `busy_timeout`, `foreign_keys` and assert expected values (`wal`, `2`, `5000`, `1`)
    - Simulate `Open` failure after DB open but before migration completes; verify `db.Close()` called and no file handle leaked

---

- [x] TASK-007: Implement AppendObservation
  - Requirements: REQ-001, REQ-002, REQ-005, REQ-006, NFR-001, NFR-002, NFR-004, NFR-005, SEC-001, SEC-002
  - Design: Proposed design — AppendObservation flow; Default application; Append-only enforcement; Context propagation and transaction semantics
  - Depends on: TASK-006, TASK-003
  - Files:
    - `store.go` (add `AppendObservation` method to `store`)
  - Completion criteria:
    - Validates all required caller-supplied fields; returns `ErrInvalidObservation` for missing/empty fields or strings exceeding max lengths
    - Validates `payload` with `json.Valid`; returns `ErrInvalidObservation` for invalid JSON
    - Rejects payload > 1 MiB (byte length); returns `ErrPayloadTooLarge`
    - Applies defaults: `visibility = "private"` if empty; `provenance.TrustLevel = "unverified"` if empty
    - Assigns `received_at = clock().UTC()` after validation passes
    - Computes `content_hash` via `hash.Canonical`; returns error if `Canonical` fails (e.g., malformed trailing JSON data)
    - Calls `repository.Insert(ctx, obs, contentHash)` and receives `(Receipt, error)`
    - Returns the `Receipt` and `error` from `repository.Insert` directly; no hash comparison or idempotency logic in the store layer
    - `store.AppendObservation` does not call `BeginTx`, `Commit`, or `Rollback` directly; all transaction management, idempotency detection, and conflict detection are delegated to `repository.Insert`
    - Payload stored as raw input bytes; not re-normalised before storage
  - Verification: `go test ./...`

---

- [x] TASK-008: Implement GetObservation and QueryObservations
  - Requirements: REQ-003, REQ-004, NFR-002, COMP-001
  - Design: Proposed design — GetObservation flow; QueryObservations flow
  - Depends on: TASK-006
  - Files:
    - `store.go` (add `GetObservation` and `QueryObservations` methods to `store`)
  - Completion criteria:
    - `GetObservation` returns `(*Observation, nil)` on hit; `(nil, ErrNotFound)` on miss; `(nil, ctx.Err())` on cancellation
    - `QueryObservations` rejects `Filter.Limit < 0` with `ErrInvalidFilter`
    - `QueryObservations` normalises limit (0→100, >1000→1000)
    - Results ordered `received_at DESC, event_id DESC`
    - All filter conditions applied with AND; unset fields ignored
    - `Page.NextCursor` always `""`
  - Verification: `go test ./...`

---

- [x] TASK-009: Write tests for AppendObservation
  - Requirements: REQ-001, REQ-002, REQ-005, REQ-006, NFR-001, NFR-002, NFR-004, NFR-005, SEC-001, SEC-002
  - Design: Test strategy — REQ-001, REQ-002, REQ-005 rows
  - Depends on: TASK-007
  - Files:
    - `store_append_test.go`
  - Completion criteria:
    - All tests below are implemented and pass
  - Verification: `go test ./...`
  - Test cases:
    - REQ-001 AC-1: `AppendObservation` returns `Receipt` with `event_id`, `received_at`, `content_hash`
    - REQ-001 AC-2: Observation survives `store.Close()` + `Open()` on file-based DB and is retrievable
    - REQ-001 AC-3: Cancelled context (before commit) returns `ctx.Err()`; `GetObservation` confirms no row written
    - REQ-001 AC-4: Payload of exactly 1 MiB (1,048,576 bytes) is accepted and stored
    - REQ-001 AC-4: Payload of 1 MiB + 1 byte (1,048,577 bytes) returns `ErrPayloadTooLarge`
    - REQ-001 AC-5 (per field): each required field omitted individually returns `ErrInvalidObservation`
    - REQ-001 AC-5 (invalid JSON): invalid JSON payload returns `ErrInvalidObservation`
    - REQ-001 AC-5 (max length): each string field exceeding max length returns `ErrInvalidObservation`
    - REQ-002 AC-1: Re-submit identical observation → same `Receipt`; `GetObservation` returns one row
    - REQ-002 AC-2: Re-submit with same `event_id` but different payload → `ErrConflict`; stored row unchanged
    - REQ-002 AC-2 (visibility/provenance): Re-submit with same `event_id` but different `visibility` → `ErrConflict`; re-submit with different `provenance.origin` → `ErrConflict`
    - REQ-002 AC-2 (default equivalence): Submit without `visibility`; re-submit with `visibility = "private"` explicitly → idempotent success (same hash as defaulted value)
    - REQ-002 AC-2 (default equivalence): Submit without `provenance.trust_level`; re-submit with `trust_level = "unverified"` explicitly → idempotent success
    - REQ-002 AC-3/4: Re-submit returns original `received_at` (injected clock advances between calls)
    - REQ-005 (reflect check): `reflect.TypeOf(InputObservation{}).FieldByName("ReceivedAt")` returns `ok == false`; test fails at runtime if `ReceivedAt` is added to `InputObservation`
    - REQ-005 AC-3: Injected clock returns fixed time; `received_at` in `Receipt` matches exactly
    - SEC-001: `visibility` defaults to `"private"` when not provided
    - SEC-002: `trust_level` defaults to `"unverified"` when not provided
    - NFR-004 (UTC roundtrip): `observed_at` with non-UTC timezone input stored and returned as UTC nanosecond-precision value matching the original instant
    - NFR-004 (nanosecond precision): Store and retrieve `received_at` and `observed_at` with sub-second values; confirm nanosecond component is preserved

---

- [x] TASK-010: Write tests for GetObservation and QueryObservations
  - Requirements: REQ-003, REQ-004, COMP-001
  - Design: Test strategy — REQ-003, REQ-004 rows
  - Depends on: TASK-008
  - Files:
    - `store_query_test.go`
  - Completion criteria:
    - All tests below are implemented and pass
  - Verification: `go test ./...`
  - Test cases:
    - REQ-003 AC-1: `GetObservation` returns stored `Observation`
    - REQ-003 AC-2: `GetObservation` with unknown `event_id` returns `ErrNotFound`; `errors.Is(err, ErrNotFound)` is true
    - REQ-003 AC-3: Cancelled context returns `ctx.Err()`
    - REQ-004 AC-1: Results ordered `received_at DESC, event_id DESC`
    - REQ-004 AC-1 (same received_at): Insert two observations with identical `received_at` (via injected clock); assert they are ordered by `event_id DESC`
    - REQ-004 AC-2: `Limit = 0` → at most 100 rows returned
    - REQ-004 AC-3: `Limit = 9999` → at most 1000 rows returned
    - REQ-004 AC-4: `Filter.SourceID` filters by exact match
    - REQ-004 AC-5: `Filter.EventType` filters by exact match
    - REQ-004 AC-6: `Filter.From`/`To` time range filters by `received_at` inclusive
    - REQ-004 AC-6 (From > To): `Filter.From` is after `Filter.To` → returns empty `Page` (zero rows match the impossible range)
    - REQ-004 AC-7: Multiple filter fields combined with AND
    - REQ-004 AC-8: No matching rows → empty `Page` with zero items
    - REQ-004 AC-9: `Page.NextCursor` is always `""`
    - Negative limit: `Filter.Limit = -1` returns `ErrInvalidFilter`

---

- [x] TASK-011: Write integration and hash canonicalisation tests
  - Requirements: REQ-007, NFR-000, REQ-002
  - Design: Test strategy — REQ-007, NFR-000, Hash canonicalisation, PRAGMA verification, Connection leak, NFR-001 rows
  - Depends on: TASK-006, TASK-003
  - Files:
    - `internal/sqlite/migrator_test.go` (extended)
    - `internal/hash/hash_test.go` (completed in TASK-003, verified here)
    - `open_test.go`
  - Completion criteria:
    - All tests below are implemented and pass
  - Verification: `go test ./...`
  - Test cases:
    - REQ-007 AC-1: Fresh file-based DB; `Migrate` called; `schema_migrations` has version 1; `observations` table exists
    - REQ-007 AC-2: Re-open same DB; `Migrate` called; version row not duplicated
    - REQ-007 AC-3: Injected failing migration; transaction rolled back; schema unchanged
    - NFR-000: After `Open` on file-based DB; `PRAGMA journal_mode` → `"wal"`
    - PRAGMA verification: After `Open`, query `synchronous` → `2` (FULL), `busy_timeout` → `5000`, `foreign_keys` → `1`
    - Connection leak: Inject migration failure in `Open`; assert `db.Close()` is called; no file handle or connection leaked
    - NFR-001: Cancel context during transaction; `GetObservation` confirms no partial row
    - Hash — byte-identical inputs → same hash
    - Hash — whitespace-only diff → same hash
    - Hash — key-order diff at any nesting level → same hash
    - Hash — numeric representation diff (`1` vs `1.0`) → different hash
    - Hash — duplicate keys → deterministic hash; idempotency holds
    - Hash — trailing data after valid JSON → `Canonical` returns error

---

- [x] TASK-012: Write README and security notice
  - Requirements: SEC-001, SEC-002, SEC-003; Security notice for integrators (design.md)
  - Design: Security considerations — Security notice for integrators
  - Depends on: TASK-006
  - Files:
    - `README.md`
  - Completion criteria:
    - README describes the library purpose, installation, and basic usage (`Open`, `AppendObservation`, `GetObservation`, `QueryObservations`)
    - Security notice section includes:
      - Millefeuille is designed to be called from a trusted application layer; it performs no authentication or authorisation
      - `visibility: "private"` is a metadata label; Millefeuille does not enforce access control on this field
      - Do not expose `Store` to untrusted callers without an authorisation layer (e.g., Guardian)
      - The SQLite database file, `-wal`, and `-shm` files must be protected with appropriate filesystem permissions by the hosting application
    - `go vet ./...` passes
  - Verification: `go vet ./...`

---

## Traceability matrix

| Requirement | Design section | Task | Test |
|---|---|---|---|
| REQ-001 AC-1 | AppendObservation flow | TASK-007 | TASK-009: Receipt fields |
| REQ-001 AC-2 | AppendObservation flow (durable) | TASK-007 | TASK-009: survives restart |
| REQ-001 AC-3 | Context propagation | TASK-007 | TASK-009: cancelled ctx |
| REQ-001 AC-4 (boundary) | Validation; Error handling | TASK-007 | TASK-009: exactly 1 MiB accepted; 1 MiB+1 rejected |
| REQ-001 AC-5 | Validation; Error handling | TASK-007 | TASK-009: per-field, invalid JSON, max length |
| REQ-002 AC-1 | Idempotency | TASK-007 | TASK-009: idempotent re-append |
| REQ-002 AC-2 | Idempotency | TASK-007 | TASK-009: conflict on payload, visibility, provenance change |
| REQ-002 AC-2 (default equiv.) | Idempotency; Default application | TASK-007 | TASK-009: default == explicit same value |
| REQ-002 AC-3/4 | Idempotency; content hash | TASK-003, TASK-007 | TASK-009: original received_at |
| REQ-003 AC-1 | GetObservation flow | TASK-008 | TASK-010: found |
| REQ-003 AC-2 | GetObservation flow; Error handling | TASK-008 | TASK-010: ErrNotFound + errors.Is |
| REQ-003 AC-3 | Context propagation | TASK-008 | TASK-010: cancelled ctx |
| REQ-004 AC-1 | QueryObservations flow | TASK-008 | TASK-010: order; same received_at → event_id DESC |
| REQ-004 AC-2 | QueryObservations flow | TASK-008 | TASK-010: default limit |
| REQ-004 AC-3 | QueryObservations flow | TASK-008 | TASK-010: limit cap |
| REQ-004 AC-4–7 | QueryObservations flow | TASK-008 | TASK-010: filter combinations; From>To empty page |
| REQ-004 AC-8 | QueryObservations flow | TASK-008 | TASK-010: empty page |
| REQ-004 AC-9 | Data-model changes — Page type | TASK-002, TASK-008 | TASK-010: next_cursor="" |
| REQ-005 | Data-model changes — Go types | TASK-002 | TASK-009: reflect runtime check (`FieldByName` returns not-found) |
| REQ-005 AC-3 | received_at assignment | TASK-007 | TASK-009: fixed clock |
| REQ-005 AC-4 | ClockFunc | TASK-002, TASK-006 | TASK-009: injectable clock |
| REQ-005 AC-6 | Validation; string field lengths | TASK-007 | TASK-009: max length per field |
| REQ-006 | Append-only enforcement | TASK-002 | Compile-time: no Update/Delete on Store |
| REQ-007 AC-1–3 | Migrator | TASK-004 | TASK-011: migration apply/skip/rollback |
| NFR-000 | SQLite connection management | TASK-006 | TASK-011: WAL + all PRAGMAs verified |
| NFR-001 | Transactional write integrity | TASK-005 (tx owner), TASK-007 | TASK-011: no partial row on cancel |
| NFR-002 | Context propagation | TASK-005 (tx), TASK-007, TASK-008 | TASK-009, TASK-010: ctx.Err() |
| NFR-003 | Parameterized queries | TASK-005 | Code review |
| NFR-004 | UTC timestamp storage | TASK-005 | TASK-009: UTC roundtrip; nanosecond precision |
| NFR-005 | Payload size enforcement | TASK-007 | TASK-009: boundary 1 MiB / 1 MiB+1 |
| COMP-001 | Page type — NextCursor | TASK-002 | TASK-010: next_cursor="" |
| COMP-002 | Store interface | TASK-002 | Compile-time |
| SEC-001 | Default application | TASK-007 | TASK-009: visibility default |
| SEC-002 | Default application | TASK-007 | TASK-009: trust_level default |
| SEC-003 | Parameterized queries | TASK-005 | Code review; static analysis |
| ErrInvalidFilter (Limit<0) | QueryObservations flow | TASK-008 | TASK-010: negative limit |
| Hash canonicalisation | hash package | TASK-003 | TASK-003 + TASK-011: all hash cases incl. trailing data |
| PRAGMA verification | SQLite connection management | TASK-006 | TASK-011: per-PRAGMA assertion |
| Connection leak | Open() | TASK-006 | TASK-011: no leak on Open failure |
| Security notice | Security considerations | TASK-012 | README review |
