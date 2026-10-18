CREATE TABLE source_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), source_id uuid NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    reviewer_id text NOT NULL, reviewer_role text NOT NULL CHECK (reviewer_role IN ('cultural','regional_language','botanical','admin')),
    decision text NOT NULL CHECK (decision IN ('approved','changes_requested','rejected')),
    notes text NOT NULL DEFAULT '', reviewed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX source_reviews_latest_idx ON source_reviews(source_id, reviewed_at DESC, id DESC);

CREATE FUNCTION prevent_append_only_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'workflow history is append-only'; END;
$$;
CREATE TRIGGER mapping_revision_evidence_immutable BEFORE UPDATE OR DELETE ON mapping_revision_evidence FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();
CREATE TRIGGER mapping_reviews_immutable BEFORE UPDATE OR DELETE ON mapping_reviews FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();
CREATE TRIGGER botanical_reviews_immutable BEFORE UPDATE OR DELETE ON botanical_reviews FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();
CREATE TRIGGER botanical_candidate_evidence_immutable BEFORE UPDATE OR DELETE ON botanical_candidate_evidence FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();
CREATE TRIGGER publication_events_immutable BEFORE UPDATE OR DELETE ON publication_events FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();
CREATE TRIGGER audit_events_immutable BEFORE UPDATE OR DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();
CREATE TRIGGER source_reviews_immutable BEFORE UPDATE OR DELETE ON source_reviews FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();

CREATE FUNCTION guard_publication_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  approved boolean;
  evidence_ok boolean;
  last_event text;
BEGIN
  IF NEW.action = 'published' THEN
    SELECT EXISTS (
      SELECT 1 FROM mapping_reviews r
      WHERE r.revision_id = NEW.revision_id AND r.decision = 'approved'
        AND r.reviewer_role IN ('cultural','regional_language')
    ) INTO approved;
    SELECT EXISTS (
      SELECT 1 FROM mapping_revision_evidence mre
      JOIN source_evidence se ON se.id = mre.evidence_id
      JOIN sources s ON s.id = se.source_id
      WHERE mre.revision_id = NEW.revision_id
        AND s.verification_status = 'verified'
        AND s.reuse_status IN ('permitted','citation_only')
        AND (se.original_text IS NULL OR (s.reuse_status = 'permitted' AND se.translation_rights IN ('permitted','not_applicable')))
    ) INTO evidence_ok;
    IF NOT approved OR NOT evidence_ok THEN
      RAISE EXCEPTION 'publication gate failed: revision requires approval and verified, rights-cleared source evidence';
    END IF;
  END IF;
  SELECT action INTO last_event FROM publication_events WHERE mapping_id = NEW.mapping_id ORDER BY occurred_at DESC, id DESC LIMIT 1;
  IF NEW.action IN ('unpublished','withdrawn') AND COALESCE(last_event, '') <> 'published' THEN
    RAISE EXCEPTION 'only a currently published mapping can be unpublished or withdrawn';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER publication_event_gate BEFORE INSERT ON publication_events FOR EACH ROW EXECUTE FUNCTION guard_publication_event();
