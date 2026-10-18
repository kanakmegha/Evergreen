-- name: ListPublishedByNakshatra :many
SELECT mr.id AS revision_id, mr.mapping_id, mr.version, t.slug AS tradition_slug,
       t.display_name AS tradition_name, tl.region_name, tl.variant_label,
       n.canonical_name AS nakshatra, mr.original_claim, mr.interpretation,
       pe.action AS publication_action
FROM mapping_revisions mr
JOIN cultural_mappings cm ON cm.id = mr.mapping_id
JOIN traditions t ON t.id = mr.tradition_id
LEFT JOIN tradition_locales tl ON tl.id = mr.locale_id
JOIN nakshatras n ON n.id = mr.nakshatra_id
JOIN LATERAL (
  SELECT action, revision_id FROM publication_events
  WHERE mapping_id = mr.mapping_id ORDER BY occurred_at DESC, id DESC LIMIT 1
) pe ON pe.action = 'published' AND pe.revision_id = mr.id
JOIN mapping_reviews r ON r.revision_id = mr.id AND r.reviewer_role IN ('cultural','regional_language') AND r.decision = 'approved'
  AND NOT EXISTS (SELECT 1 FROM mapping_reviews newer WHERE newer.revision_id=r.revision_id AND (newer.reviewed_at,newer.id)>(r.reviewed_at,r.id))
WHERE n.canonical_name = sqlc.arg(nakshatra)::text
  AND t.slug = sqlc.arg(tradition_slug)::text
  AND EXISTS (
    SELECT 1 FROM mapping_revision_evidence mre
    JOIN source_evidence se ON se.id = mre.evidence_id
    JOIN sources s ON s.id = se.source_id
    WHERE mre.revision_id = mr.id AND s.reuse_status IN ('permitted','citation_only')
  )
ORDER BY COALESCE(tl.region_name, ''), mr.mapping_id, mr.version;

-- name: ListRevisionEvidence :many
SELECT se.id, s.title, s.author, s.publisher, s.publication_date, s.url,
       s.bibliographic_reference, s.language, s.accessed_on, s.attribution_requirements,
       s.reuse_status, se.locator, se.original_text, se.language AS passage_language,
       mre.relationship
FROM mapping_revision_evidence mre
JOIN source_evidence se ON se.id = mre.evidence_id
JOIN sources s ON s.id = se.source_id
WHERE mre.revision_id = sqlc.arg(revision_id)::uuid
ORDER BY s.title, se.locator;

-- name: ListBotanicalCandidates :many
SELECT bc.id, ts.scientific_name, bc.certainty, bc.confidence,
       (SELECT br.decision FROM botanical_reviews br WHERE br.candidate_id = bc.id
        ORDER BY br.reviewed_at DESC, br.id DESC LIMIT 1) AS latest_decision
FROM botanical_candidates bc JOIN tree_species ts ON ts.id = bc.species_id
WHERE bc.revision_id = sqlc.arg(revision_id)::uuid
ORDER BY bc.created_at, bc.id;

-- name: ListReviewQueue :many
SELECT mr.id AS revision_id, mr.mapping_id, mr.version, t.display_name AS tradition,
       COALESCE(tl.region_name, '') AS region, n.canonical_name AS nakshatra,
       mr.original_claim, mr.created_at
FROM mapping_revisions mr
JOIN traditions t ON t.id = mr.tradition_id
LEFT JOIN tradition_locales tl ON tl.id = mr.locale_id
JOIN nakshatras n ON n.id = mr.nakshatra_id
WHERE NOT EXISTS (SELECT 1 FROM mapping_reviews r WHERE r.revision_id = mr.id AND r.decision = 'approved'
  AND NOT EXISTS (SELECT 1 FROM mapping_reviews newer WHERE newer.revision_id=r.revision_id AND (newer.reviewed_at,newer.id)>(r.reviewed_at,r.id)))
ORDER BY mr.created_at, mr.id;

-- name: RecordMappingReview :exec
INSERT INTO mapping_reviews (revision_id, reviewer_id, reviewer_role, decision, notes)
VALUES (sqlc.arg(revision_id)::uuid, sqlc.arg(reviewer_id)::text,
        sqlc.arg(reviewer_role)::text, sqlc.arg(decision)::text, sqlc.arg(notes)::text);

-- name: RecordPublicationEvent :exec
INSERT INTO publication_events (mapping_id, revision_id, action, actor_id, reason)
VALUES (sqlc.arg(mapping_id)::uuid, sqlc.arg(revision_id)::uuid,
        sqlc.arg(action)::text, sqlc.arg(actor_id)::text, sqlc.arg(reason)::text);
