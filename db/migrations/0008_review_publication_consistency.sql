CREATE FUNCTION prevent_review_of_published_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE active_revision uuid; active_action text;
BEGIN
  SELECT revision_id, action INTO active_revision, active_action FROM publication_events
    WHERE mapping_id=(SELECT mapping_id FROM mapping_revisions WHERE id=NEW.revision_id)
    ORDER BY occurred_at DESC,id DESC LIMIT 1;
  IF active_revision=NEW.revision_id AND active_action='published' THEN
    RAISE EXCEPTION 'published revisions cannot be re-reviewed; submit a new revision or withdraw publication';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER prevent_published_revision_review BEFORE INSERT ON mapping_reviews
FOR EACH ROW EXECUTE FUNCTION prevent_review_of_published_revision();
