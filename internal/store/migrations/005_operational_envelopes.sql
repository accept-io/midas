CREATE TABLE operational_envelopes (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    surface_id TEXT,
    surface_version INTEGER,
    profile_id TEXT,
    profile_version INTEGER,
    agent_id TEXT,
    state TEXT NOT NULL,
    outcome TEXT,
    reason_code TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    closed_at TIMESTAMPTZ,
    FOREIGN KEY (surface_id, surface_version)
        REFERENCES decision_surfaces(id, version),
    FOREIGN KEY (profile_id, profile_version)
        REFERENCES authority_profiles(id, version)
);

CREATE INDEX idx_operational_envelopes_request_id
    ON operational_envelopes (request_id);

CREATE INDEX idx_operational_envelopes_state
    ON operational_envelopes (state);