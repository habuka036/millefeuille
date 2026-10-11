# Millefeuille — Stage 0 integration contract

Status: Draft v0.1. Millefeuille is an **independent, reusable memory project**, not an IMAGORIUM submodule. Canonical IMAGORIUM integration design: `https://github.com/imagorium/imago-agent/tree/main/docs/plans/stage-0` (may be private; do not copy restricted material into a public repo).

## Minimal API
- `AppendObservation(ctx, Observation) -> Receipt`: durable append with unique event_id and idempotent identical replay.
- `GetObservation(ctx, eventID) -> Observation | NotFound`.
- `QueryObservations(ctx, Filter{deviceID,eventType,timeRange,limit,cursor}) -> Page` (simple bounded query; cursor may be deferred to M0.2).

## Storage
SQLite adapter, transactionally persisted events, stable payload hash, source and visibility labels, device and receive timestamps, no overwriting raw observations. Treat source-provided confidence as untrusted; do not automatically infer validated knowledge. No hard dependency on Guardian or IMAGO.

## Acceptance
Append/read/restart; identical replay no duplication; conflicting event_id rejected; bad schema rejected; bounded queries; database migration tests. No embedding search or memory promotion in Stage 0.

## Privacy and access
Keep private memory data private by default. This is an application-level label, not an OS-enforced isolation guarantee. Do not assume `private` metadata alone prevents unauthorized reads; the hosting application must enforce authentication and authorization at the boundary.
