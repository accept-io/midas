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
    required_context_keys JSONB,
    version INTEGER NOT NULL,
    effective_date TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (id, version)
);