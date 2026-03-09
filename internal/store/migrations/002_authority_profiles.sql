CREATE TABLE authority_profiles (
    id TEXT NOT NULL,
    surface_id TEXT NOT NULL,
    surface_version INTEGER NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    max_confidence_threshold NUMERIC(5,4),
    max_consequence_type TEXT,
    max_consequence_amount NUMERIC(18,2),
    max_consequence_currency TEXT,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    effective_date TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id, version),
    FOREIGN KEY (surface_id, surface_version)
        REFERENCES decision_surfaces(id, version)
);