CREATE TRIGGER source_evidence_immutable BEFORE UPDATE OR DELETE ON source_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();
CREATE TRIGGER botanical_candidates_immutable BEFORE UPDATE OR DELETE ON botanical_candidates
FOR EACH ROW EXECUTE FUNCTION prevent_append_only_mutation();

CREATE FUNCTION ensure_botanical_candidate_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM botanical_candidate_evidence WHERE candidate_id=NEW.id) THEN
    RAISE EXCEPTION 'botanical candidates require independent source evidence';
  END IF;
  RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER botanical_candidate_requires_evidence
AFTER INSERT ON botanical_candidates DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION ensure_botanical_candidate_evidence();

CREATE FUNCTION prevent_source_metadata_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF ROW(NEW.title,NEW.source_type,NEW.author,NEW.publisher,NEW.publication_date,NEW.url,
         NEW.bibliographic_reference,NEW.language,NEW.accessed_on,NEW.attribution_requirements,NEW.reuse_status)
     IS DISTINCT FROM
     ROW(OLD.title,OLD.source_type,OLD.author,OLD.publisher,OLD.publication_date,OLD.url,
         OLD.bibliographic_reference,OLD.language,OLD.accessed_on,OLD.attribution_requirements,OLD.reuse_status) THEN
    RAISE EXCEPTION 'source metadata is immutable; register a corrected source and create a new mapping revision';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER source_metadata_immutable BEFORE UPDATE ON sources
FOR EACH ROW EXECUTE FUNCTION prevent_source_metadata_mutation();

CREATE FUNCTION prevent_species_mutation_when_referenced() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM botanical_candidates WHERE species_id=OLD.id) THEN
    RAISE EXCEPTION 'referenced botanical identities are immutable; create a new species record and mapping revision';
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER referenced_species_immutable BEFORE UPDATE OR DELETE ON tree_species
FOR EACH ROW EXECUTE FUNCTION prevent_species_mutation_when_referenced();

CREATE FUNCTION require_source_review_for_status_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE latest_decision text;
BEGIN
  IF NEW.verification_status IS DISTINCT FROM OLD.verification_status THEN
    SELECT decision INTO latest_decision FROM source_reviews WHERE source_id=OLD.id
      ORDER BY reviewed_at DESC,id DESC LIMIT 1;
    IF (NEW.verification_status='verified' AND COALESCE(latest_decision,'')<>'approved')
       OR (NEW.verification_status='rejected' AND COALESCE(latest_decision,'')<>'rejected')
       OR (NEW.verification_status='unverified' AND COALESCE(latest_decision,'')<>'changes_requested') THEN
      RAISE EXCEPTION 'source verification status must follow an audited source review';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER source_status_review_gate BEFORE UPDATE OF verification_status ON sources
FOR EACH ROW EXECUTE FUNCTION require_source_review_for_status_change();
