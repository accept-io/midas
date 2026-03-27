-- MIDAS Database Schema v2.1
-- Hardened schema for MIDAS authority governance engine
--
-- This file is the single source of truth for the database structure.
-- It is applied by EnsureSchema (internal/store/postgres/schema.go) at startup
-- and by setup-db.sh for local developer setup.
--
-- All DDL is written to be idempotent:
--   - CREATE TABLE IF NOT EXISTS
--   - CREATE [UNIQUE] INDEX IF NOT EXISTS
--   - CREATE OR REPLACE VIEW
-- FK constraints are declared inline in CREATE TABLE rather than as separate
-- ALTER TABLE statements (which have no IF NOT EXISTS in Postgres).

-- =============================================================================
-- DECISION SURFACES
-- =============================================================================
-- id is the LOGICAL surface identifier across versions.
-- A concrete version is identified by (id, version).
-- Runtime resolution chooses the correct version by status + effective dates.

CREATE TABLE IF NOT EXISTS decision_surfaces (
    -- Composite logical-version key
    id TEXT NOT NULL,
    version INTEGER NOT NULL,

    -- Core evaluation fields
    name TEXT NOT NULL,
    description TEXT,
    domain TEXT NOT NULL,
    category TEXT,
    taxonomy JSONB NOT NULL DEFAULT '[]',
    tags JSONB NOT NULL DEFAULT '[]',

    decision_type TEXT,
    reversibility_class TEXT,

    required_context JSONB NOT NULL DEFAULT '{"fields":[]}',
    consequence_types JSONB NOT NULL DEFAULT '[]',
    minimum_confidence DOUBLE PRECISION NOT NULL DEFAULT 0.0,

    policy_package TEXT,
    policy_version TEXT,
    failure_mode TEXT NOT NULL DEFAULT 'closed',

    mandatory_evidence JSONB NOT NULL DEFAULT '[]',
    audit_retention_hours INTEGER NOT NULL DEFAULT 0,
    subject_required BOOLEAN NOT NULL DEFAULT false,
    compliance_frameworks JSONB NOT NULL DEFAULT '[]',

    -- Lifecycle
    status TEXT NOT NULL,
    effective_date TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ,
    deprecation_reason TEXT,
    successor_surface_id TEXT,
    successor_version INTEGER,

    -- Ownership
    business_owner TEXT NOT NULL,
    technical_owner TEXT NOT NULL,
    stakeholders JSONB NOT NULL DEFAULT '[]',

    -- Documentation
    documentation_url TEXT,
    external_references JSONB NOT NULL DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    created_by TEXT,
    approved_by TEXT,
    approved_at TIMESTAMPTZ,

    PRIMARY KEY (id, version),

    -- Optional self-reference for successor version
    CONSTRAINT fk_surfaces_successor
        FOREIGN KEY (successor_surface_id, successor_version)
        REFERENCES decision_surfaces (id, version)
        DEFERRABLE INITIALLY DEFERRED,

    CONSTRAINT chk_surfaces_status
        CHECK (status IN ('draft', 'review', 'active', 'deprecated', 'retired')),

    CONSTRAINT chk_surfaces_min_confidence
        CHECK (minimum_confidence >= 0.0 AND minimum_confidence <= 1.0),

    CONSTRAINT chk_surfaces_decision_type
        CHECK (
            decision_type IS NULL
            OR decision_type IN ('strategic', 'tactical', 'operational')
        ),

    CONSTRAINT chk_surfaces_reversibility
        CHECK (
            reversibility_class IS NULL
            OR reversibility_class IN ('reversible', 'conditionally_reversible', 'irreversible')
        ),

    CONSTRAINT chk_surfaces_failure_mode
        CHECK (failure_mode IN ('open', 'closed')),

    CONSTRAINT chk_surfaces_audit_retention
        CHECK (audit_retention_hours = 0 OR audit_retention_hours >= 24),

    CONSTRAINT chk_surfaces_effective_dates
        CHECK (effective_until IS NULL OR effective_until > effective_date),

    CONSTRAINT chk_surfaces_approval_fields
        CHECK (
            (status IN ('draft', 'review') AND approved_at IS NULL)
            OR
            (status IN ('active', 'deprecated', 'retired'))
        )
);

CREATE INDEX IF NOT EXISTS idx_decision_surfaces_id_version_desc
    ON decision_surfaces (id, version DESC);

CREATE INDEX IF NOT EXISTS idx_decision_surfaces_status
    ON decision_surfaces (status);

CREATE INDEX IF NOT EXISTS idx_decision_surfaces_domain
    ON decision_surfaces (domain);

CREATE INDEX IF NOT EXISTS idx_decision_surfaces_effective_date
    ON decision_surfaces (effective_date);

CREATE INDEX IF NOT EXISTS idx_decision_surfaces_active
    ON decision_surfaces (id, effective_date, effective_until)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_decision_surfaces_taxonomy_gin
    ON decision_surfaces USING GIN (taxonomy jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_decision_surfaces_tags_gin
    ON decision_surfaces USING GIN (tags jsonb_path_ops);

-- =============================================================================
-- AUTHORITY PROFILES
-- =============================================================================
-- id is the LOGICAL profile identifier across versions.
-- A concrete version is identified by (id, version).
-- Grants reference the logical profile id; runtime resolution selects the
-- correct profile version using status + effective dates.
--
-- NOTE: surface_id references the LOGICAL ID of decision_surfaces, not a
-- concrete version. This cannot be enforced with a normal FK because
-- decision_surfaces is keyed by (id, version). Runtime logic resolves the
-- correct active surface version for evaluation.

CREATE TABLE IF NOT EXISTS authority_profiles (
    -- Composite logical-version key
    id TEXT NOT NULL,
    version INTEGER NOT NULL,

    -- Linkage to logical decision surface
    surface_id TEXT NOT NULL,

    -- Profile metadata
    name TEXT NOT NULL,
    description TEXT,

    -- Lifecycle
    status TEXT NOT NULL,
    effective_date TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ,
    retired_at TIMESTAMPTZ,

    -- Threshold configuration
    confidence_threshold DOUBLE PRECISION NOT NULL,

    -- Consequence threshold tagged union
    consequence_type TEXT NOT NULL,
    consequence_amount NUMERIC(18,2),
    consequence_currency TEXT,
    consequence_risk_rating TEXT,

    -- Policy integration
    policy_reference TEXT,

    -- Governance handling semantics
    escalation_mode TEXT NOT NULL,
    fail_mode TEXT NOT NULL,

    -- Context requirements
    required_context_keys JSONB NOT NULL DEFAULT '[]',

    -- Extensibility
    delegation_policy JSONB NOT NULL DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    created_by TEXT,
    approved_by TEXT,
    approved_at TIMESTAMPTZ,

    PRIMARY KEY (id, version),

    CONSTRAINT chk_profiles_status
        CHECK (status IN ('draft', 'review', 'active', 'deprecated', 'retired')),

    CONSTRAINT chk_profiles_confidence
        CHECK (confidence_threshold >= 0.0 AND confidence_threshold <= 1.0),

    CONSTRAINT chk_profiles_consequence_type
        CHECK (consequence_type IN ('risk_rating', 'financial', 'temporal', 'impact_scope', 'custom')),

    CONSTRAINT chk_profiles_consequence_risk_rating
        CHECK (
            consequence_risk_rating IS NULL
            OR consequence_risk_rating IN ('low', 'medium', 'high', 'critical')
        ),

    CONSTRAINT chk_profiles_escalation_mode
        CHECK (escalation_mode IN ('auto', 'manual')),

    CONSTRAINT chk_profiles_fail_mode
        CHECK (fail_mode IN ('open', 'closed')),

    CONSTRAINT chk_profiles_effective_dates
        CHECK (
            effective_until IS NULL OR effective_until > effective_date
        ),

    CONSTRAINT chk_profiles_retired_at
        CHECK (
            retired_at IS NULL OR retired_at >= effective_date
        ),

    CONSTRAINT chk_profiles_approval_fields
        CHECK (
            (status IN ('draft', 'review') AND approved_at IS NULL)
            OR
            (status IN ('active', 'deprecated', 'retired'))
        ),

    -- Tagged-union enforcement
    CONSTRAINT chk_profiles_consequence_union
        CHECK (
            (
                consequence_type = 'financial'
                AND consequence_amount IS NOT NULL
                AND consequence_currency IS NOT NULL
                AND consequence_risk_rating IS NULL
            )
            OR
            (
                consequence_type = 'risk_rating'
                AND consequence_amount IS NULL
                AND consequence_currency IS NULL
                AND consequence_risk_rating IS NOT NULL
            )
            OR
            (
                consequence_type IN ('temporal', 'impact_scope', 'custom')
            )
        )
);

CREATE INDEX IF NOT EXISTS idx_authority_profiles_id_version_desc
    ON authority_profiles (id, version DESC);

CREATE INDEX IF NOT EXISTS idx_authority_profiles_surface_id
    ON authority_profiles (surface_id);

CREATE INDEX IF NOT EXISTS idx_authority_profiles_status
    ON authority_profiles (status);

CREATE INDEX IF NOT EXISTS idx_authority_profiles_effective_date
    ON authority_profiles (effective_date);

CREATE INDEX IF NOT EXISTS idx_authority_profiles_active
    ON authority_profiles (id, effective_date, effective_until)
    WHERE status = 'active';

-- =============================================================================
-- AGENTS
-- =============================================================================

CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,

    owner TEXT,
    created_by TEXT,

    model_version TEXT,
    endpoint TEXT,
    capabilities JSONB NOT NULL DEFAULT '[]',

    operational_state TEXT NOT NULL,

    metadata JSONB NOT NULL DEFAULT '{}',

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT chk_agents_type
        CHECK (type IN ('ai', 'human', 'system', 'hybrid')),

    CONSTRAINT chk_agents_operational_state
        CHECK (operational_state IN ('active', 'suspended', 'retired'))
);

CREATE INDEX IF NOT EXISTS idx_agents_operational_state
    ON agents (operational_state);

CREATE INDEX IF NOT EXISTS idx_agents_type
    ON agents (type);

CREATE INDEX IF NOT EXISTS idx_agents_owner
    ON agents (owner);

CREATE INDEX IF NOT EXISTS idx_agents_capabilities_gin
    ON agents USING GIN (capabilities jsonb_path_ops);

-- =============================================================================
-- AUTHORITY GRANTS
-- =============================================================================
-- Grants link an agent to a LOGICAL authority profile id.
-- Runtime resolution then chooses the correct active profile version.
-- Grants themselves should be treated as non-deletable operational history;
-- revocation/suspension should be modeled by status and revocation fields.
--
-- Grants may participate in delegation chains for authority-graph lineage
-- tracking and audit/reporting. parent_grant_id is a self-reference to the
-- grant from which this one was delegated. authority_chain_id groups all
-- grants in a lineage chain under a single stable identifier. delegation_depth
-- records the number of hops from the root grant (0 = root).

CREATE TABLE IF NOT EXISTS authority_grants (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    profile_id TEXT NOT NULL,

    granted_by TEXT,
    grant_reason TEXT,
    status TEXT NOT NULL,

    effective_date TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoked_by TEXT,
    revocation_reason TEXT,
    suspended_at TIMESTAMPTZ,
    suspended_by TEXT,
    suspend_reason TEXT,

    -- Delegation and authority-graph lineage
    parent_grant_id           TEXT,
    authority_chain_id        TEXT,
    delegation_depth          INTEGER NOT NULL DEFAULT 0,
    delegated_from_agent_id   TEXT,
    delegated_from_profile_id TEXT,

    CONSTRAINT fk_grants_agent
        FOREIGN KEY (agent_id) REFERENCES agents(id),

    CONSTRAINT fk_grants_parent_grant
        FOREIGN KEY (parent_grant_id) REFERENCES authority_grants(id),

    CONSTRAINT chk_grants_status
        CHECK (status IN ('active', 'suspended', 'revoked')),

    CONSTRAINT chk_grants_dates
        CHECK (expires_at IS NULL OR expires_at > effective_date),

    CONSTRAINT chk_grants_revocation_fields
        CHECK (
            (status = 'revoked' AND revoked_at IS NOT NULL)
            OR
            (status IN ('active', 'suspended'))
        )
);

CREATE INDEX IF NOT EXISTS idx_authority_grants_agent_id
    ON authority_grants (agent_id);

CREATE INDEX IF NOT EXISTS idx_authority_grants_profile_id
    ON authority_grants (profile_id);

CREATE INDEX IF NOT EXISTS idx_authority_grants_status
    ON authority_grants (status);

CREATE INDEX IF NOT EXISTS idx_authority_grants_agent_status
    ON authority_grants (agent_id, status);

CREATE INDEX IF NOT EXISTS idx_authority_grants_effective
    ON authority_grants (effective_date, expires_at)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_authority_grants_parent_grant
    ON authority_grants (parent_grant_id);

CREATE INDEX IF NOT EXISTS idx_authority_grants_chain_id
    ON authority_grants (authority_chain_id);

CREATE INDEX IF NOT EXISTS idx_authority_grants_delegation_depth
    ON authority_grants (delegation_depth);

-- =============================================================================
-- OPERATIONAL ENVELOPES
-- =============================================================================
-- Denormalized authority-chain identifiers are stored as top-level columns for:
-- - explainability
-- - reporting
-- - query performance
-- - simpler joins
--
-- JSON sections still carry the richer detail.

CREATE TABLE IF NOT EXISTS operational_envelopes (
    id TEXT PRIMARY KEY,

    -- Idempotency scoping
    request_source TEXT NOT NULL,
    request_id TEXT NOT NULL,

    schema_version INTEGER NOT NULL DEFAULT 1,
    state TEXT NOT NULL,

    outcome TEXT,
    reason_code TEXT,

    -- SECTION 1: SUBMITTED
    submitted_raw JSONB,
    submitted_hash TEXT,
    received_at TIMESTAMPTZ,

    -- SECTION 2: RESOLVED (denormalized authority chain)
    resolved_surface_id TEXT,
    resolved_surface_version INTEGER,
    resolved_profile_id TEXT,
    resolved_profile_version INTEGER,
    resolved_grant_id TEXT,
    resolved_agent_id TEXT,
    resolved_subject_id TEXT,

    resolved_json JSONB NOT NULL DEFAULT '{}',

    -- SECTION 3: EVALUATION
    evaluated_at TIMESTAMPTZ,
    explanation_json JSONB,

    -- SECTION 4: INTEGRITY
    integrity_json JSONB NOT NULL DEFAULT '{}',

    -- SECTION 5: REVIEW
    review_json JSONB,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    closed_at TIMESTAMPTZ,

    CONSTRAINT fk_envelopes_resolved_agent
        FOREIGN KEY (resolved_agent_id)
        REFERENCES agents(id),

    CONSTRAINT fk_envelopes_resolved_grant
        FOREIGN KEY (resolved_grant_id)
        REFERENCES authority_grants(id),

    CONSTRAINT fk_envelopes_resolved_surface
        FOREIGN KEY (resolved_surface_id, resolved_surface_version)
        REFERENCES decision_surfaces(id, version),

    CONSTRAINT fk_envelopes_resolved_profile
        FOREIGN KEY (resolved_profile_id, resolved_profile_version)
        REFERENCES authority_profiles(id, version),

    CONSTRAINT chk_envelopes_state
        CHECK (state IN ('received', 'evaluating', 'escalated', 'outcome_recorded', 'awaiting_review', 'closed')),

    CONSTRAINT chk_envelopes_outcome
        CHECK (
            outcome IS NULL
            OR outcome IN ('accept', 'escalate', 'reject', 'request_clarification')
        ),

    CONSTRAINT chk_envelopes_schema_version
        CHECK (schema_version >= 1),

    CONSTRAINT chk_envelopes_closed_state
        CHECK (
            (state = 'closed' AND closed_at IS NOT NULL)
            OR
            (state <> 'closed')
        ),

    CONSTRAINT chk_envelopes_review_semantics
        CHECK (
            review_json IS NULL OR outcome = 'escalate'
        ),

    CONSTRAINT chk_envelopes_evaluated_at
        CHECK (
            evaluated_at IS NULL OR received_at IS NULL OR evaluated_at >= received_at
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_operational_envelopes_request_scope
    ON operational_envelopes (request_source, request_id);

CREATE INDEX IF NOT EXISTS idx_envelopes_state
    ON operational_envelopes (state);

CREATE INDEX IF NOT EXISTS idx_envelopes_outcome
    ON operational_envelopes (outcome)
    WHERE outcome IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_envelopes_closed_at
    ON operational_envelopes (closed_at)
    WHERE closed_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_envelopes_created_at
    ON operational_envelopes (created_at);

CREATE INDEX IF NOT EXISTS idx_envelopes_resolved_surface
    ON operational_envelopes (resolved_surface_id, resolved_surface_version);

CREATE INDEX IF NOT EXISTS idx_envelopes_resolved_profile
    ON operational_envelopes (resolved_profile_id, resolved_profile_version);

CREATE INDEX IF NOT EXISTS idx_envelopes_resolved_agent
    ON operational_envelopes (resolved_agent_id);

CREATE INDEX IF NOT EXISTS idx_envelopes_resolved_grant
    ON operational_envelopes (resolved_grant_id);

CREATE INDEX IF NOT EXISTS idx_envelopes_resolved_subject
    ON operational_envelopes (resolved_subject_id)
    WHERE resolved_subject_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_envelopes_submitted_hash
    ON operational_envelopes (submitted_hash)
    WHERE submitted_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_envelopes_resolved_gin
    ON operational_envelopes USING GIN (resolved_json jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_envelopes_explanation_gin
    ON operational_envelopes USING GIN (explanation_json jsonb_path_ops);

-- =============================================================================
-- AUDIT EVENTS
-- =============================================================================
-- Append-only event log.
-- Application role should receive SELECT + INSERT only.
-- For stronger immutability, add a trigger rejecting UPDATE/DELETE.

CREATE TABLE IF NOT EXISTS audit_events (
    id TEXT PRIMARY KEY,
    envelope_id TEXT NOT NULL,
    request_source TEXT NOT NULL,
    request_id TEXT NOT NULL,
    sequence_no INTEGER NOT NULL,

    occurred_at TIMESTAMPTZ NOT NULL,
    event_type TEXT NOT NULL,

    performed_by_type TEXT NOT NULL,
    performed_by_id TEXT NOT NULL,

    payload_json JSONB,

    prev_hash TEXT,
    event_hash TEXT NOT NULL,

    CONSTRAINT fk_audit_events_envelope
        FOREIGN KEY (envelope_id) REFERENCES operational_envelopes(id),

    UNIQUE (envelope_id, sequence_no),

    CONSTRAINT chk_audit_performed_by_type
        CHECK (performed_by_type IN ('system', 'agent', 'human', 'api'))
);

CREATE INDEX IF NOT EXISTS idx_audit_events_envelope_id_seq
    ON audit_events (envelope_id, sequence_no);

CREATE INDEX IF NOT EXISTS idx_audit_events_request_scope
    ON audit_events (request_source, request_id);

CREATE INDEX IF NOT EXISTS idx_audit_events_occurred_at
    ON audit_events (occurred_at);

CREATE INDEX IF NOT EXISTS idx_audit_events_event_type
    ON audit_events (event_type);

CREATE INDEX IF NOT EXISTS idx_audit_events_performer
    ON audit_events (performed_by_type, performed_by_id);

CREATE INDEX IF NOT EXISTS idx_audit_events_hash_chain
    ON audit_events (envelope_id, sequence_no, prev_hash, event_hash);

CREATE INDEX IF NOT EXISTS idx_audit_events_payload_gin
    ON audit_events USING GIN (payload_json jsonb_path_ops);

-- =============================================================================
-- OUTBOX EVENTS
-- =============================================================================
-- Transactional outbox for reliable event delivery to downstream consumers.
--
-- Each row is written in the same database transaction as the domain state
-- change that produced it. A dispatcher reads unpublished rows and delivers
-- them to downstream systems, then marks them published. If the domain
-- transaction rolls back, the outbox row is removed along with it.
--
-- Audit events and outbox events are separate concerns:
--   - audit_events: hash-chained, append-only governance records.
--   - outbox_events: routing envelopes for downstream integration.

CREATE TABLE IF NOT EXISTS outbox_events (
    id             TEXT PRIMARY KEY,
    event_type     TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id   TEXT NOT NULL,
    topic          TEXT NOT NULL,
    event_key      TEXT,
    payload        JSONB NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL,
    published_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_unpublished
    ON outbox_events (created_at ASC)
    WHERE published_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_outbox_events_aggregate
    ON outbox_events (aggregate_type, aggregate_id);

CREATE INDEX IF NOT EXISTS idx_outbox_events_event_type
    ON outbox_events (event_type);

COMMENT ON TABLE outbox_events IS
'Transactional outbox: domain integration events written atomically with domain state. Dispatcher marks rows published after delivery.';

-- =============================================================================
-- CONTROL-PLANE CONFIGURATION AUDIT TRAIL
-- =============================================================================
-- controlplane_audit_events is a separate, append-only table for control-plane
-- governance history. It is distinct from audit_events (runtime decision audit).
-- Records capture who changed what, when, and which version of which resource.

CREATE TABLE IF NOT EXISTS controlplane_audit_events (
    id               TEXT        NOT NULL PRIMARY KEY,
    occurred_at      TIMESTAMPTZ NOT NULL,
    actor            TEXT        NOT NULL,
    action           TEXT        NOT NULL CHECK (action IN (
                         'surface.created',
                         'profile.created',
                         'profile.versioned',
                         'agent.created',
                         'grant.created',
                         'surface.approved',
                         'surface.deprecated',
                         'profile.approved',
                         'profile.deprecated',
                         'grant.suspended',
                         'grant.revoked',
                         'grant.reinstated'
                     )),
    resource_kind    TEXT        NOT NULL CHECK (resource_kind IN ('surface', 'profile', 'agent', 'grant')),
    resource_id      TEXT        NOT NULL,
    resource_version INTEGER,
    summary          TEXT        NOT NULL,
    metadata         JSONB
);

CREATE INDEX IF NOT EXISTS idx_cp_audit_occurred_at    ON controlplane_audit_events (occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_cp_audit_resource_kind  ON controlplane_audit_events (resource_kind);
CREATE INDEX IF NOT EXISTS idx_cp_audit_resource_id    ON controlplane_audit_events (resource_id);
CREATE INDEX IF NOT EXISTS idx_cp_audit_actor          ON controlplane_audit_events (actor);
CREATE INDEX IF NOT EXISTS idx_cp_audit_action         ON controlplane_audit_events (action);

-- =============================================================================
-- VIEWS
-- =============================================================================

CREATE OR REPLACE VIEW v_surfaces_latest AS
SELECT DISTINCT ON (id) *
FROM decision_surfaces
ORDER BY id, version DESC;

CREATE OR REPLACE VIEW v_surfaces_active AS
SELECT *
FROM decision_surfaces
WHERE status = 'active'
  AND effective_date <= NOW()
  AND (effective_until IS NULL OR effective_until > NOW());

CREATE OR REPLACE VIEW v_profiles_latest AS
SELECT DISTINCT ON (id) *
FROM authority_profiles
ORDER BY id, version DESC;

CREATE OR REPLACE VIEW v_profiles_active AS
SELECT *
FROM authority_profiles
WHERE status = 'active'
  AND effective_date <= NOW()
  AND (effective_until IS NULL OR effective_until > NOW());

CREATE OR REPLACE VIEW v_grants_active AS
SELECT *
FROM authority_grants
WHERE status = 'active'
  AND effective_date <= NOW()
  AND (expires_at IS NULL OR expires_at > NOW());

CREATE OR REPLACE VIEW v_envelopes_open AS
SELECT *
FROM operational_envelopes
WHERE state <> 'closed';

CREATE OR REPLACE VIEW v_envelopes_awaiting_review AS
SELECT *
FROM operational_envelopes
WHERE state = 'awaiting_review';

-- =============================================================================
-- COMMENTS
-- =============================================================================

COMMENT ON TABLE decision_surfaces IS
'Versioned decision surface definitions. id is the logical surface ID; (id, version) identifies a concrete version.';

COMMENT ON TABLE authority_profiles IS
'Versioned authority profiles. id is the logical profile ID; (id, version) identifies a concrete version.';

COMMENT ON TABLE authority_grants IS
'Links agents to logical authority profiles. Grants participate in delegation chains for authority-graph lineage tracking and audit/reporting. Grants should be revoked/suspended rather than deleted.';

COMMENT ON TABLE operational_envelopes IS
'Five-section governance envelope wrapping each request through submission, authority resolution, evaluation, integrity tracking, and review.';

COMMENT ON TABLE audit_events IS
'Append-only immutable-style audit log with per-envelope hash chain integrity.';

COMMENT ON COLUMN operational_envelopes.resolved_surface_id IS
'Logical surface ID resolved for this request.';

COMMENT ON COLUMN operational_envelopes.resolved_surface_version IS
'Concrete surface version resolved for this request.';

COMMENT ON COLUMN operational_envelopes.resolved_profile_id IS
'Logical profile ID resolved for this request.';

COMMENT ON COLUMN operational_envelopes.resolved_profile_version IS
'Concrete profile version resolved for this request.';

COMMENT ON COLUMN audit_events.event_hash IS
'SHA-256 hash over event material including prev_hash to form a tamper-evident per-envelope chain.';

COMMENT ON TABLE controlplane_audit_events IS
'Append-only control-plane governance audit trail. Records who changed what, when, and which version.';

COMMENT ON COLUMN controlplane_audit_events.action IS
'Typed action constant: surface.created | profile.created | profile.versioned | agent.created | grant.created | surface.approved | surface.deprecated | profile.approved | profile.deprecated | grant.suspended | grant.revoked | grant.reinstated';

COMMENT ON COLUMN controlplane_audit_events.resource_version IS
'Version of the resource at the time of the action. Null for resources without versioning (agent, grant).';

COMMENT ON COLUMN controlplane_audit_events.metadata IS
'Typed JSON metadata: surface_id for profiles, deprecation_reason and successor_surface_id for deprecation.';

-- =============================================================================
-- OPTIONAL HARDENING TRIGGER FOR AUDIT IMMUTABILITY
-- =============================================================================
-- Uncomment if you want database-level rejection of UPDATE/DELETE on audit_events.

-- CREATE OR REPLACE FUNCTION reject_audit_events_mutation()
-- RETURNS trigger AS $$
-- BEGIN
--   RAISE EXCEPTION 'audit_events is append-only; UPDATE/DELETE not allowed';
-- END;
-- $$ LANGUAGE plpgsql;
--
-- CREATE TRIGGER trg_reject_audit_events_update
-- BEFORE UPDATE ON audit_events
-- FOR EACH ROW
-- EXECUTE FUNCTION reject_audit_events_mutation();
--
-- CREATE TRIGGER trg_reject_audit_events_delete
-- BEFORE DELETE ON audit_events
-- FOR EACH ROW
-- EXECUTE FUNCTION reject_audit_events_mutation();

-- =============================================================================
-- EXAMPLE PRIVILEGES
-- =============================================================================

-- GRANT SELECT, INSERT, UPDATE ON decision_surfaces TO midas_app;
-- GRANT SELECT, INSERT, UPDATE ON authority_profiles TO midas_app;
-- GRANT SELECT, INSERT, UPDATE ON agents TO midas_app;
-- GRANT SELECT, INSERT, UPDATE ON authority_grants TO midas_app;
-- GRANT SELECT, INSERT, UPDATE ON operational_envelopes TO midas_app;
-- GRANT SELECT, INSERT ON audit_events TO midas_app;
-- GRANT SELECT ON
--   v_surfaces_latest,
--   v_surfaces_active,
--   v_profiles_latest,
--   v_profiles_active,
--   v_grants_active,
--   v_envelopes_open,
--   v_envelopes_awaiting_review
-- TO midas_app;
