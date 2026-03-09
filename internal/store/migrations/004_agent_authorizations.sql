CREATE TABLE agent_authorizations (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    granted_by TEXT,
    status TEXT NOT NULL,
    effective_date TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_agent_authorizations_agent_id
    ON agent_authorizations (agent_id);

CREATE INDEX idx_agent_authorizations_profile_id
    ON agent_authorizations (profile_id);

CREATE INDEX idx_agent_authorizations_status
    ON agent_authorizations (status);