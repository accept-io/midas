CREATE TABLE decision_surfaces (
    id TEXT NOT NULL,
    name TEXT NOT NULL,
    domain TEXT NOT NULL,
    business_owner TEXT,
    technical_owner TEXT,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    effective_date TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (id, version)
);