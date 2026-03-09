CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    envelope_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    sequence_no INTEGER NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    event_type TEXT NOT NULL,
    performed_by_type TEXT NOT NULL,
    performed_by_id TEXT NOT NULL,
    payload_json JSONB,
    prev_hash TEXT,
    event_hash TEXT NOT NULL,
    UNIQUE (envelope_id, sequence_no)
);

CREATE INDEX idx_audit_events_envelope_id_seq
    ON audit_events (envelope_id, sequence_no);

CREATE INDEX idx_audit_events_request_id
    ON audit_events (request_id);

CREATE INDEX idx_audit_events_occurred_at
    ON audit_events (occurred_at);