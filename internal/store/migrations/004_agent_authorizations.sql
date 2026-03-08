CREATE TABLE agent_authorizations (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    granted_by TEXT,
    effective_date TIMESTAMP NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);