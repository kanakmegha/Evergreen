-- Require every linked cultural evidence item to be verified and rights-cleared.
CREATE OR REPLACE FUNCTION guard_publication_evidence_rights() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE latest_action text; latest_revision uuid; newest_revision uuid;
BEGIN
  IF NEW.action = 'published' THEN
    SELECT mr.id INTO newest_revision FROM mapping_revisions mr
      WHERE mr.mapping_id=NEW.mapping_id ORDER BY mr.version DESC LIMIT 1;
    IF newest_revision IS DISTINCT FROM NEW.revision_id THEN
      RAISE EXCEPTION 'only the newest immutable mapping revision may be published';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM mapping_revision_evidence WHERE revision_id=NEW.revision_id) THEN
      RAISE EXCEPTION 'published mappings require source evidence';
    END IF;
    IF EXISTS (
      SELECT 1 FROM mapping_revision_evidence mre
      JOIN source_evidence se ON se.id=mre.evidence_id
      JOIN sources s ON s.id=se.source_id
      WHERE mre.revision_id=NEW.revision_id
        AND (s.verification_status <> 'verified'
          OR s.reuse_status NOT IN ('permitted','citation_only')
          OR (se.original_text IS NOT NULL AND s.reuse_status <> 'permitted')
          OR (se.translation IS NOT NULL AND se.translation_rights <> 'permitted'))
    ) THEN
      RAISE EXCEPTION 'all linked evidence must have verified sources and documented reuse rights';
    END IF;
  ELSE
    SELECT action, revision_id INTO latest_action, latest_revision FROM publication_events
      WHERE mapping_id=NEW.mapping_id ORDER BY occurred_at DESC, id DESC LIMIT 1;
    IF COALESCE(latest_action,'') <> 'published' OR latest_revision IS DISTINCT FROM NEW.revision_id THEN
      RAISE EXCEPTION 'only the currently published revision can be unpublished or withdrawn';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER publication_integrity_gate BEFORE INSERT ON publication_events
FOR EACH ROW EXECUTE FUNCTION guard_publication_evidence_rights();
