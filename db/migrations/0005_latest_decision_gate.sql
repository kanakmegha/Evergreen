CREATE FUNCTION guard_latest_mapping_review() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE latest_decision text; latest_role text;
BEGIN
  IF NEW.action = 'published' THEN
    SELECT decision, reviewer_role INTO latest_decision, latest_role
      FROM mapping_reviews WHERE revision_id=NEW.revision_id
      ORDER BY reviewed_at DESC, id DESC LIMIT 1;
    IF COALESCE(latest_decision,'') <> 'approved' OR COALESCE(latest_role,'') NOT IN ('cultural','regional_language') THEN
      RAISE EXCEPTION 'publication requires the latest review decision for this exact revision to be approved';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER latest_mapping_review_gate BEFORE INSERT ON publication_events
FOR EACH ROW EXECUTE FUNCTION guard_latest_mapping_review();
