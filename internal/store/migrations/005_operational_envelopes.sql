CREATE TABLE operational_envelopes (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    surface_id TEXT NOT NULL,
    surface_version INTEGER NOT NULL,
    state TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    closed_at TIMESTAMPTZ,
    FOREIGN KEY (surface_id, surface_version)
        REFERENCES decision_surfaces(id, version)
);

CREATE INDEX idx_operational_envelopes_request_id
    ON operational_envelopes (request_id);

CREATE INDEX idx_operational_envelopes_state
    ON operational_envelopes (state);