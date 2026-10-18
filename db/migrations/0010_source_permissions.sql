-- Everroot source permission registry.
--
-- This migration adds a permission registry layer on top of the existing
-- `sources` table (0001_registry.sql). It records, per source and per material
-- type, the human-reviewed permission state and the conditions under which
-- material may be collected, stored, or published.
--
-- All workflow history tables are append-only. Corrections are recorded as new
-- rows, never as in-place edits, consistent with the existing immutability model
-- in migrations 0001-0009.
--
-- Permission states (controlled vocabulary):
--   allowed_under_licence   - recorded licence covers the proposed use
--   written_permission_required - rightsholder confirmation needed before use
--   restricted              - use is restricted by the rightsholder
--   unknown                 - rights could not be assessed; treat as not cleared
--   public_domain_verified  - verified public domain (requires specific evidence)

CREATE TABLE source_permission_material_types (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    material_type text NOT NULL CHECK (material_type IN (
        'botanical_data',
        'cultural_claims',
        'original_prose',
        'creative_compilation',
        'translations',
        'photographs_illustrations',
        'community_traditional_knowledge'
    )),
    permission_state text NOT NULL CHECK (permission_state IN (
        'allowed_under_licence',
        'written_permission_required',
        'restricted',
        'unknown',
        'public_domain_verified'
    )),
    licence_name text,
    licence_version text,
    licence_url text,
    licence_clauses text,
    commercial_use_allowed boolean NOT NULL DEFAULT false,
    extraction_allowed boolean NOT NULL DEFAULT false,
    local_storage_allowed boolean NOT NULL DEFAULT false,
    redistribution_allowed boolean NOT NULL DEFAULT false,
    adaptation_allowed boolean NOT NULL DEFAULT false,
    translation_allowed boolean NOT NULL DEFAULT false,
    attribution_required boolean NOT NULL DEFAULT true,
    attribution_requirements text,
    share_alike_required boolean NOT NULL DEFAULT false,
    third_party_rights text,
    automated_access_permitted boolean NOT NULL DEFAULT false,
    written_permission_required boolean NOT NULL DEFAULT true,
    written_permission_on_file boolean NOT NULL DEFAULT false,
    community_consent_required boolean NOT NULL DEFAULT false,
    community_consent_on_file boolean NOT NULL DEFAULT false,
    allowed_operations text[] NOT NULL DEFAULT '{}'::text[],
    reviewed_by text NOT NULL,
    reviewed_at timestamptz NOT NULL DEFAULT now(),
    review_notes text,
    evidence_urls text[] NOT NULL DEFAULT '{}'::text[],
    next_review_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(source_id, material_type)
);

CREATE INDEX source_permission_material_types_source_idx ON source_permission_material_types(source_id);
CREATE INDEX source_permission_material_types_state_idx ON source_permission_material_types(permission_state);

-- Append-only record of every permission-gate evaluation for reproducibility.
CREATE TABLE permission_gate_decisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    material_type text NOT NULL,
    operation text NOT NULL,
    is_commercial boolean NOT NULL DEFAULT false,
    attribution_met boolean NOT NULL DEFAULT false,
    community_consent_met boolean NOT NULL DEFAULT false,
    decision text NOT NULL CHECK (decision IN ('allowed','blocked')),
    reasons text[] NOT NULL DEFAULT '{}'::text[],
    decided_by text NOT NULL DEFAULT 'system',
    decided_at timestamptz NOT NULL DEFAULT now(),
    request_id text NOT NULL DEFAULT ''
);

CREATE INDEX permission_gate_decisions_source_idx ON permission_gate_decisions(source_id, decided_at DESC);

-- Append-only record of human overrides of the permission gate.
-- An override is never silent: it must carry legal grounds, evidence, reviewer,
-- timestamp, and scope.
CREATE TABLE permission_overrides (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    material_type text NOT NULL,
    operation text NOT NULL,
    legal_grounds text NOT NULL,
    supporting_evidence text NOT NULL,
    reviewer_id text NOT NULL,
    reviewer_role text NOT NULL,
    scope text NOT NULL,
    expires_at timestamptz,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    request_id text NOT NULL DEFAULT ''
);

CREATE INDEX permission_overrides_source_idx ON permission_overrides(source_id, recorded_at DESC);

-- Guard: overrides are append-only.
CREATE TRIGGER permission_overrides_immutable
BEFORE UPDATE OR DELETE ON permission_overrides
FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();

-- Guard: gate decisions are append-only.
CREATE TRIGGER permission_gate_decisions_immutable
BEFORE UPDATE OR DELETE ON permission_gate_decisions
FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();

-- Guard: permission material-type rows are immutable once reviewed.
CREATE TRIGGER source_permission_material_types_immutable
BEFORE UPDATE OR DELETE ON source_permission_material_types
FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();
