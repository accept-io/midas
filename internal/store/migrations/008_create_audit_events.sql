CREATE TABLE audit_events (
    id text PRIMARY KEY,
    envelope_id text NOT NULL,
    request_id text NOT NULL,
    sequence_no integer NOT NULL,
    event_type text NOT NULL,
    performed_by_type text,
    performed_by_id text,
    payload_json jsonb NOT NULL,
    prev_hash text,
    event_hash text NOT NULL,
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (envelope_id, sequence_no)
);

CREATE INDEX idx_audit_events_envelope_id_seq
    ON audit_events (envelope_id, sequence_no);

CREATE INDEX idx_audit_events_request_id
    ON audit_events (request_id);

CREATE INDEX idx_audit_events_occurred_at
    ON audit_events (occurred_at);