CREATE FUNCTION guard_botanical_review_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.decision = 'approved' AND NOT EXISTS (
    SELECT 1 FROM botanical_candidate_evidence bce
    JOIN source_evidence se ON se.id=bce.evidence_id
    JOIN sources s ON s.id=se.source_id
    WHERE bce.candidate_id=NEW.candidate_id
      AND s.verification_status='verified'
      AND s.reuse_status IN ('permitted','citation_only')
      AND (se.original_text IS NULL OR s.reuse_status='permitted')
      AND (se.translation IS NULL OR se.translation_rights='permitted')
  ) THEN
    RAISE EXCEPTION 'botanical approval requires verified, rights-cleared botanical evidence';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER botanical_review_evidence_gate BEFORE INSERT ON botanical_reviews
FOR EACH ROW EXECUTE FUNCTION guard_botanical_review_evidence();
