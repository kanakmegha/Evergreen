-- Everroot registry foundation. A cultural revision is immutable: edits create a new version.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE cultures (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    display_name text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived'))
);
CREATE TABLE traditions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), culture_id uuid NOT NULL REFERENCES cultures(id) ON DELETE RESTRICT,
    slug text NOT NULL, display_name text NOT NULL, status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    UNIQUE(culture_id, slug)
);
CREATE TABLE tradition_locales (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tradition_id uuid NOT NULL REFERENCES traditions(id) ON DELETE RESTRICT,
    locale text NOT NULL, region_code text NOT NULL, region_name text NOT NULL, variant_label text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    UNIQUE(tradition_id, locale, region_code, variant_label)
);
CREATE TABLE nakshatras (
    id smallint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, canonical_name text NOT NULL UNIQUE,
    ordinal smallint NOT NULL UNIQUE CHECK (ordinal BETWEEN 1 AND 27)
);
CREATE TABLE nakshatra_names (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), nakshatra_id smallint NOT NULL REFERENCES nakshatras(id) ON DELETE RESTRICT,
    name text NOT NULL, locale text NOT NULL, script text NOT NULL, UNIQUE(nakshatra_id, name, locale)
);

CREATE TABLE sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), title text NOT NULL, source_type text NOT NULL,
    author text NOT NULL DEFAULT '', publisher text NOT NULL DEFAULT '', publication_date date,
    url text NOT NULL DEFAULT '', bibliographic_reference text NOT NULL DEFAULT '', language text NOT NULL,
    accessed_on date NOT NULL, attribution_requirements text NOT NULL DEFAULT '',
    reuse_status text NOT NULL CHECK (reuse_status IN ('permitted','citation_only','restricted','unknown')),
    verification_status text NOT NULL DEFAULT 'unverified' CHECK (verification_status IN ('unverified','verified','rejected')),
    created_at timestamptz NOT NULL DEFAULT now(), CHECK (url <> '' OR bibliographic_reference <> '')
);
CREATE TABLE source_evidence (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), source_id uuid NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    locator text NOT NULL, original_text text, language text NOT NULL, translation text,
    translation_rights text NOT NULL DEFAULT 'not_applicable' CHECK (translation_rights IN ('permitted','citation_only','restricted','unknown','not_applicable')),
    evidence_notes text NOT NULL DEFAULT '', CHECK (original_text IS NULL OR length(btrim(original_text)) > 0)
);

CREATE TABLE tree_species (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), scientific_name text NOT NULL UNIQUE,
    family text NOT NULL DEFAULT '', botanical_status text NOT NULL DEFAULT 'unverified' CHECK (botanical_status IN ('unverified','verified','rejected'))
);
CREATE TABLE tree_names (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), species_id uuid NOT NULL REFERENCES tree_species(id) ON DELETE RESTRICT,
    name text NOT NULL, language text NOT NULL, region_code text NOT NULL DEFAULT '', name_type text NOT NULL CHECK (name_type IN ('common','regional','scientific_synonym')),
    evidence_id uuid REFERENCES source_evidence(id) ON DELETE RESTRICT
);

CREATE TABLE cultural_mappings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE mapping_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), mapping_id uuid NOT NULL REFERENCES cultural_mappings(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version > 0), tradition_id uuid NOT NULL REFERENCES traditions(id) ON DELETE RESTRICT,
    locale_id uuid REFERENCES tradition_locales(id) ON DELETE RESTRICT,
    rule_type text NOT NULL CHECK (rule_type = 'birth_nakshatra'), nakshatra_id smallint NOT NULL REFERENCES nakshatras(id) ON DELETE RESTRICT,
    original_claim text NOT NULL CHECK (length(btrim(original_claim)) > 0), interpretation text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(mapping_id, version), UNIQUE(mapping_id, id)
);
CREATE TABLE mapping_revision_evidence (
    revision_id uuid NOT NULL REFERENCES mapping_revisions(id) ON DELETE RESTRICT,
    evidence_id uuid NOT NULL REFERENCES source_evidence(id) ON DELETE RESTRICT,
    relationship text NOT NULL CHECK (relationship IN ('supports','qualifies','contradicts','context')),
    PRIMARY KEY(revision_id, evidence_id, relationship)
);
CREATE TABLE mapping_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), revision_id uuid NOT NULL REFERENCES mapping_revisions(id) ON DELETE RESTRICT,
    reviewer_id text NOT NULL, reviewer_role text NOT NULL CHECK (reviewer_role IN ('cultural','regional_language','botanical','admin')),
    decision text NOT NULL CHECK (decision IN ('approved','changes_requested','rejected')),
    notes text NOT NULL DEFAULT '', reviewed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mapping_reviews_revision_idx ON mapping_reviews(revision_id, reviewed_at DESC);
CREATE TABLE botanical_candidates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), revision_id uuid NOT NULL REFERENCES mapping_revisions(id) ON DELETE RESTRICT,
    species_id uuid NOT NULL REFERENCES tree_species(id) ON DELETE RESTRICT,
    certainty text NOT NULL CHECK (certainty IN ('uncertain','probable','confirmed')),
    confidence numeric(4,3) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(revision_id, species_id)
);
CREATE TABLE botanical_candidate_evidence (
    candidate_id uuid NOT NULL REFERENCES botanical_candidates(id) ON DELETE RESTRICT,
    evidence_id uuid NOT NULL REFERENCES source_evidence(id) ON DELETE RESTRICT,
    relationship text NOT NULL CHECK (relationship IN ('identifies','supports','qualifies','contradicts')),
    PRIMARY KEY(candidate_id, evidence_id, relationship)
);
CREATE TABLE botanical_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), candidate_id uuid NOT NULL REFERENCES botanical_candidates(id) ON DELETE RESTRICT,
    reviewer_id text NOT NULL, reviewer_role text NOT NULL CHECK (reviewer_role IN ('botanical','admin')),
    decision text NOT NULL CHECK (decision IN ('approved','changes_requested','rejected')),
    notes text NOT NULL DEFAULT '', reviewed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE publication_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), mapping_id uuid NOT NULL, revision_id uuid NOT NULL,
    action text NOT NULL CHECK (action IN ('published','unpublished','withdrawn')),
    actor_id text NOT NULL, reason text NOT NULL DEFAULT '', occurred_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(mapping_id, revision_id) REFERENCES mapping_revisions(mapping_id, id) ON DELETE RESTRICT
);
CREATE INDEX publication_events_latest_idx ON publication_events(mapping_id, occurred_at DESC, id DESC);
CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), actor_id text NOT NULL, action text NOT NULL,
    entity_type text NOT NULL, entity_id uuid NOT NULL, revision_id uuid,
    occurred_at timestamptz NOT NULL DEFAULT now(), request_id text NOT NULL DEFAULT '', details jsonb NOT NULL DEFAULT '{}'::jsonb,
    CHECK (jsonb_typeof(details) = 'object')
);
CREATE INDEX audit_events_entity_idx ON audit_events(entity_type, entity_id, occurred_at DESC);

-- Guard immutable revisions against in-place edits. The only valid change is a new version row.
CREATE FUNCTION prevent_mapping_revision_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'mapping revisions are immutable; create a new version'; END;
$$;
CREATE TRIGGER mapping_revisions_immutable BEFORE UPDATE OR DELETE ON mapping_revisions
FOR EACH ROW EXECUTE FUNCTION prevent_mapping_revision_mutation();

-- Public view uses the latest event; unpublish/withdrawal never destroys history.
CREATE VIEW published_mapping_revisions AS
SELECT mr.* FROM mapping_revisions mr
JOIN LATERAL (
    SELECT pe.action, pe.revision_id FROM publication_events pe
    WHERE pe.mapping_id = mr.mapping_id ORDER BY pe.occurred_at DESC, pe.id DESC LIMIT 1
) latest ON latest.action = 'published' AND latest.revision_id = mr.id;
