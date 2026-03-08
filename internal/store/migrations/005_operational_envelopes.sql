CREATE TABLE operational_envelopes (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    surface_id TEXT NOT NULL,
    surface_version INTEGER,
    profile_id TEXT,
    profile_version INTEGER,
    agent_id TEXT,
    state TEXT NOT NULL,
    outcome TEXT,
    reason_code TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    closed_at TIMESTAMP
);