CREATE TABLE authority_profiles (
    id TEXT NOT NULL,
    surface_id TEXT NOT NULL,
    name TEXT NOT NULL,
    confidence_threshold DOUBLE PRECISION NOT NULL,
    consequence_type TEXT NOT NULL,
    consequence_amount DOUBLE PRECISION,
    consequence_currency TEXT,
    consequence_risk_rating TEXT,
    policy_reference TEXT,
    escalation_mode TEXT NOT NULL,
    fail_mode TEXT NOT NULL,
    required_context_keys JSONB NOT NULL,
    version INTEGER NOT NULL,
    effective_date TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id, version)
);

CREATE INDEX idx_authority_profiles_surface_id
    ON authority_profiles (surface_id);

CREATE INDEX idx_authority_profiles_effective_date
    ON authority_profiles (effective_date);