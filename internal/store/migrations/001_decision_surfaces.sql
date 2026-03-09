CREATE TABLE decision_surfaces (
    id TEXT NOT NULL,
    name TEXT NOT NULL,
    domain TEXT NOT NULL,
    business_owner TEXT,
    technical_owner TEXT,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    effective_date TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id, version)
);