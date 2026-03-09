CREATE TABLE agent_authorizations (
    id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_version INTEGER NOT NULL,
    profile_id TEXT NOT NULL,
    profile_version INTEGER NOT NULL,
    granted_by TEXT,
    status TEXT NOT NULL,
    effective_date TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (agent_id, agent_version)
        REFERENCES agents(id, version),
    FOREIGN KEY (profile_id, profile_version)
        REFERENCES authority_profiles(id, version)
);