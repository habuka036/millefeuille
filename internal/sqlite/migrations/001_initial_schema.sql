-- Migration 001: initial schema for Millefeuille Stage 0 (M0.1)

CREATE TABLE IF NOT EXISTS observations (
    event_id        TEXT    NOT NULL,
    schema_version  TEXT    NOT NULL,
    source_id       TEXT    NOT NULL,
    device_id       TEXT    NOT NULL DEFAULT '',
    event_type      TEXT    NOT NULL,
    observed_at     TEXT    NOT NULL,  -- nanosecond UTC: 2006-01-02T15:04:05.000000000Z
    received_at     TEXT    NOT NULL,  -- nanosecond UTC: 2006-01-02T15:04:05.000000000Z; library-assigned
    payload         TEXT    NOT NULL,  -- raw input JSON bytes as-is; valid JSON enforced in Go; max 1 MiB
    visibility      TEXT    NOT NULL DEFAULT 'private',
    provenance      TEXT    NOT NULL,  -- JSON: {"origin":"...","trust_level":"..."}
    content_hash    TEXT    NOT NULL,  -- hex SHA-256 of canonical projection (excludes received_at)
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
