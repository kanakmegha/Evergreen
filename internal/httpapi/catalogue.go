package httpapi

// Exploratory tree discovery.
//
// Everroot keeps two operations apart on purpose:
//
//	Track A, reviewed cultural association — served by POST /api/v1/discoveries,
//	    gated on published revisions, review decisions and rights-cleared
//	    evidence. When there is no match it returns no_verified_mapping and
//	    nothing is invented.
//	Track B, exploratory botanical discovery — served by GET /api/v1/trees and
//	    POST /api/v1/explain. It reports botanical authority records (identity,
//	    classification, nomenclatural provenance) and never derives a cultural
//	    claim from them.
//
// The AI provider is only ever asked to explain facts that were already
// retrieved from Everroot's own dataset, and its failures are reported as a
// distinct provider state, never as missing cultural data.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"everroot/internal/ai"
)

// explainer is the slice of the AI client this package needs. A nil explainer
// means the explanation layer is switched off, which is reported honestly.
type explainer interface {
	Explain(ctx context.Context, in ai.ExplainInput) (ai.ExplainResult, error)
}

// treeSource reads the botanical reference catalogue.
type treeSource interface {
	List(ctx context.Context, query string, limit, offset int) ([]treeFact, error)
	Get(ctx context.Context, speciesID string) (treeFact, error)
}

// errTreeNotFound distinguishes an unknown tree from an unavailable catalogue.
var errTreeNotFound = errors.New("tree not found")

// Bounds for the public catalogue endpoint.
const (
	maxTreePageSize      = 100
	defaultTreePageSize  = 60
	exploreGroundingSize = 3
)

// Cultural-association states. "reviewed_match" is the only one that may be
// presented as a verified cultural record.
const (
	assocReviewedMatch   = "reviewed_match"
	assocNoReviewedMatch = "no_reviewed_match"
	assocNotQueryable    = "not_queryable"
)

// Explanation states and reasons, kept as a closed vocabulary so the interface
// can tell a provider failure apart from an empty dataset.
const (
	explainOK          = "ok"
	explainUnavailable = "unavailable"

	reasonNotConfigured = "not_configured"
	reasonUnauthorized  = "unauthorized"
	reasonRateLimited   = "rate_limited"
	reasonTimeout       = "timeout"
	reasonProvider      = "provider_error"
	reasonEmpty         = "empty_response"
)

// treeCommonName is one vernacular name as supplied by the authority dataset.
type treeCommonName struct {
	Name     string `json:"name"`
	Language string `json:"language"`
}

// treeAuthorityRef points at the taxon record every fact came from.
type treeAuthorityRef struct {
	Dataset  string `json:"dataset"`
	DOI      string `json:"doi"`
	URL      string `json:"url"`
	TaxonKey int64  `json:"taxonKey"`
}

// treeFact is one entry in the botanical reference catalogue.
type treeFact struct {
	SpeciesID     string           `json:"speciesId"`
	AcceptedName  string           `json:"acceptedName"`
	Authorship    string           `json:"authorship"`
	Family        string           `json:"family"`
	Genus         string           `json:"genus"`
	Order         string           `json:"order"`
	Rank          string           `json:"rank"`
	TaxonStatus   string           `json:"taxonStatus"`
	Protologue    string           `json:"protologue"`
	CommonNames   []treeCommonName `json:"commonNames"`
	Authority     treeAuthorityRef `json:"authority"`
	Sources       []sourceResult   `json:"sources"`
	RecordType    string           `json:"recordType"`
	CulturalClaim string           `json:"culturalClaim"`
}

// evidenceStatus tells the interface exactly which parts of a selection are
// backed by a reviewed record and which are not. It is structured rather than
// prose so the wording can change without changing the honesty of the data.
type evidenceStatus struct {
	CulturalAssociation string `json:"culturalAssociation"`
	BotanicalIdentity   string `json:"botanicalIdentity"`
	Habitat             string `json:"habitat"`
	EcologicalRole      string `json:"ecologicalRole"`
	DocumentedStories   string `json:"documentedStories"`
	PersonalMeaning     string `json:"personalMeaning"`
}

// explanationEnvelope carries generated text and, on failure, a precise reason.
type explanationEnvelope struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
	Text   string `json:"text"`
	Model  string `json:"model"`
}

const listTreesSQL = `
SELECT ts.id, a.accepted_name, a.authorship, a.family, a.genus, a.order_name,
       a.rank, a.taxon_status, a.protologue,
       a.authority_dataset, a.authority_doi, a.authority_url, a.gbif_taxon_key
FROM tree_species ts
JOIN tree_species_authority a ON a.species_id = ts.id
WHERE ts.botanical_status = 'verified'
  AND a.taxon_status = 'ACCEPTED'
  AND ($1 = '' OR a.accepted_name ILIKE '%' || $1 || '%'
       OR a.family ILIKE '%' || $1 || '%'
       OR a.genus ILIKE '%' || $1 || '%'
       OR EXISTS (SELECT 1 FROM tree_names tn WHERE tn.species_id = ts.id AND tn.name ILIKE '%' || $1 || '%'))
ORDER BY a.family, a.accepted_name
LIMIT $2 OFFSET $3`

const getTreeSQL = `
SELECT ts.id, a.accepted_name, a.authorship, a.family, a.genus, a.order_name,
       a.rank, a.taxon_status, a.protologue,
       a.authority_dataset, a.authority_doi, a.authority_url, a.gbif_taxon_key
FROM tree_species ts
JOIN tree_species_authority a ON a.species_id = ts.id
WHERE ts.botanical_status = 'verified' AND a.taxon_status = 'ACCEPTED' AND ts.id = $1`

const listTreeNamesSQL = `
SELECT species_id, name, language FROM tree_names
WHERE species_id = ANY($1::uuid[])
ORDER BY species_id, language, name`

const listTreeSourcesSQL = `
SELECT x.species_id, s.id, s.title, s.author, s.publisher, s.publication_date, s.url,
       s.bibliographic_reference, s.language, s.accessed_on, s.attribution_requirements,
       s.reuse_status, se.locator, NULL::text, NULL::text, se.translation_rights,
       s.verification_status, 'identifies'::text
FROM (
    SELECT species_id, evidence_id FROM tree_species_authority WHERE species_id = ANY($1::uuid[])
    UNION
    SELECT species_id, evidence_id FROM tree_names WHERE species_id = ANY($1::uuid[]) AND evidence_id IS NOT NULL
) x
JOIN source_evidence se ON se.id = x.evidence_id
JOIN sources s ON s.id = se.source_id
WHERE s.verification_status = 'verified' AND s.reuse_status IN ('permitted', 'citation_only')
ORDER BY s.title, se.locator`

// reviewedAssociationExistsSQL is the same publication, review and rights gate
// the cultural discovery endpoint applies. Keeping the predicate identical is
// what guarantees Track B can never claim a cultural match Track A would deny.
const reviewedAssociationExistsSQL = `
SELECT EXISTS(
  SELECT 1
  FROM mapping_revisions mr
  JOIN traditions t ON t.id = mr.tradition_id
  LEFT JOIN tradition_locales tl ON tl.id = mr.locale_id
  JOIN nakshatras n ON n.id = mr.nakshatra_id
  JOIN LATERAL (
      SELECT action, revision_id FROM publication_events
      WHERE mapping_id = mr.mapping_id ORDER BY occurred_at DESC, id DESC LIMIT 1
  ) pe ON pe.action = 'published' AND pe.revision_id = mr.id
  WHERE t.slug = $1 AND lower(n.canonical_name) = lower($2)
    AND ($3 = '' OR lower(COALESCE(tl.region_name, '')) = lower($3))
    AND EXISTS (
      SELECT 1 FROM mapping_revision_evidence mre
      JOIN source_evidence se ON se.id = mre.evidence_id
      JOIN sources s ON s.id = se.source_id
      WHERE mre.revision_id = mr.id AND s.verification_status = 'verified'
        AND s.reuse_status IN ('permitted', 'citation_only')
    )
    AND NOT EXISTS (
      SELECT 1 FROM mapping_revision_evidence mre
      JOIN source_evidence se ON se.id = mre.evidence_id
      JOIN sources s ON s.id = se.source_id
      WHERE mre.revision_id = mr.id
        AND (s.verification_status <> 'verified'
          OR s.reuse_status NOT IN ('permitted', 'citation_only')
          OR (se.original_text IS NOT NULL AND s.reuse_status <> 'permitted')
          OR (se.translation IS NOT NULL AND se.translation_rights <> 'permitted'))
    )
)`

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// escapeLike neutralises wildcard characters in a user-supplied search term.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// ---------------------------------------------------------------- catalogue

// pgTreeSource reads the botanical reference catalogue from PostgreSQL.
type pgTreeSource struct {
	db *pgxpool.Pool
}

func (s pgTreeSource) rows(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.db.Query(ctx, sql, args...)
}

func (s pgTreeSource) List(ctx context.Context, query string, limit, offset int) ([]treeFact, error) {
	rows, err := s.rows(ctx, listTreesSQL, likeEscaper.Replace(strings.TrimSpace(query)), limit, offset)
	if err != nil {
		return nil, err
	}
	return s.collect(ctx, rows)
}

func (s pgTreeSource) Get(ctx context.Context, speciesID string) (treeFact, error) {
	if !uuidPattern.MatchString(speciesID) {
		return treeFact{}, errTreeNotFound
	}
	rows, err := s.rows(ctx, getTreeSQL, speciesID)
	if err != nil {
		return treeFact{}, err
	}
	facts, err := s.collect(ctx, rows)
	if err != nil {
		return treeFact{}, err
	}
	if len(facts) == 0 {
		return treeFact{}, errTreeNotFound
	}
	return facts[0], nil
}

func (s pgTreeSource) collect(ctx context.Context, rows pgx.Rows) ([]treeFact, error) {
	defer rows.Close()
	facts := []treeFact{}
	for rows.Next() {
		var f treeFact
		if err := rows.Scan(
			&f.SpeciesID, &f.AcceptedName, &f.Authorship, &f.Family, &f.Genus, &f.Order,
			&f.Rank, &f.TaxonStatus, &f.Protologue,
			&f.Authority.Dataset, &f.Authority.DOI, &f.Authority.URL, &f.Authority.TaxonKey,
		); err != nil {
			return nil, err
		}
		f.CommonNames = []treeCommonName{}
		f.Sources = []sourceResult{}
		f.RecordType = "botanical_authority"
		// A catalogue entry is a botanical record. Everroot never derives a
		// cultural claim from one, so there is never a claim attached here.
		f.CulturalClaim = "none"
		facts = append(facts, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(facts) == 0 {
		return facts, nil
	}

	ids := make([]string, 0, len(facts))
	for _, f := range facts {
		ids = append(ids, f.SpeciesID)
	}
	names := map[string][]treeCommonName{}
	nameRows, err := s.rows(ctx, listTreeNamesSQL, ids)
	if err != nil {
		return nil, err
	}
	for nameRows.Next() {
		var speciesID string
		var name treeCommonName
		if err := nameRows.Scan(&speciesID, &name.Name, &name.Language); err != nil {
			nameRows.Close()
			return nil, err
		}
		names[speciesID] = append(names[speciesID], name)
	}
	if err := nameRows.Err(); err != nil {
		nameRows.Close()
		return nil, err
	}
	nameRows.Close()

	sources := map[string][]sourceResult{}
	sourceRows, err := s.rows(ctx, listTreeSourcesSQL, ids)
	if err != nil {
		return nil, err
	}
	for sourceRows.Next() {
		var speciesID string
		var src sourceResult
		if err := sourceRows.Scan(
			&speciesID, &src.ID, &src.Title, &src.Author, &src.Publisher, &src.PublicationDate,
			&src.URL, &src.Bibliography, &src.Language, &src.AccessedOn, &src.Attribution,
			&src.Rights, &src.Locator, &src.Text, &src.Translation, &src.TranslationRights,
			&src.VerificationStatus, &src.Relationship,
		); err != nil {
			sourceRows.Close()
			return nil, err
		}
		sources[speciesID] = append(sources[speciesID], src)
	}
	if err := sourceRows.Err(); err != nil {
		sourceRows.Close()
		return nil, err
	}
	sourceRows.Close()

	for i := range facts {
		if n, ok := names[facts[i].SpeciesID]; ok {
			facts[i].CommonNames = n
		}
		if s, ok := sources[facts[i].SpeciesID]; ok {
			facts[i].Sources = s
		}
	}
	return facts, nil
}

// ---------------------------------------------------------------- handlers

func (a *API) trees(w http.ResponseWriter, r *http.Request) {
	if a.catalogue == nil {
		writeError(w, 503, "botanical catalogue unavailable")
		return
	}
	query := r.URL.Query().Get("q")
	if len(query) > 120 {
		writeError(w, 400, "search term is too long")
		return
	}
	limit := defaultTreePageSize
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxTreePageSize {
			writeError(w, 400, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, 400, "offset must be zero or greater")
			return
		}
		offset = parsed
	}

	facts, err := a.catalogue.List(r.Context(), query, limit, offset)
	if err != nil {
		a.log.Error("catalogue read failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
		writeError(w, 503, "botanical catalogue unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{
		"status": "ok",
		// Deterministic and stated, so nothing looks like a ranking.
		"ordering": "family, then accepted name",
		"query":    strings.TrimSpace(query),
		"count":    len(facts),
		"trees":    facts,
	})
}

// intentionLabels is the closed vocabulary accepted from the interface. The
// free text a person writes about their own meaning is never sent here.
var intentionLabels = map[string]string{
	"personal": "something about their own life",
	"remember": "someone they want to remember",
	"culture":  "their own culture, traditions or beliefs",
	"future":   "something meant to last for someone else",
	"explore":  "open curiosity, nothing personal shared",
}

type explainRequest struct {
	Intention string `json:"intention"`
	Tradition string `json:"tradition"`
	Nakshatra string `json:"nakshatra"`
	Region    string `json:"region"`
	SpeciesID string `json:"speciesId"`
}

func (a *API) explain(w http.ResponseWriter, r *http.Request) {
	if a.catalogue == nil {
		writeError(w, 503, "botanical catalogue unavailable")
		return
	}
	var req explainRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid JSON request")
		return
	}
	req.Tradition = strings.TrimSpace(req.Tradition)
	req.Nakshatra = strings.TrimSpace(req.Nakshatra)
	req.Region = strings.TrimSpace(req.Region)
	req.SpeciesID = strings.TrimSpace(req.SpeciesID)

	label, ok := intentionLabels[req.Intention]
	if !ok {
		writeError(w, 400, "unsupported intention")
		return
	}
	if len(req.Tradition) > 100 || len(req.Nakshatra) > 100 || len(req.Region) > 100 || len(req.SpeciesID) > 64 {
		writeError(w, 400, "invalid selection")
		return
	}

	ctx := r.Context()
	task := ai.TaskExploreSelection

	var species []treeFact
	if req.SpeciesID != "" {
		task = ai.TaskWhyThisTree
		fact, err := a.catalogue.Get(ctx, req.SpeciesID)
		if errors.Is(err, errTreeNotFound) {
			writeError(w, 404, "no botanical record for that tree")
			return
		}
		if err != nil {
			a.log.Error("catalogue read failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
			writeError(w, 503, "botanical catalogue unavailable")
			return
		}
		species = []treeFact{fact}
	} else {
		loaded, err := a.catalogue.List(ctx, "", exploreGroundingSize, 0)
		if err != nil {
			a.log.Error("catalogue read failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
			writeError(w, 503, "botanical catalogue unavailable")
			return
		}
		species = loaded
	}

	// Track A is evaluated with the real publication gate. Track B never
	// asserts a cultural link; this is the only place a match can be claimed.
	association := assocNotQueryable
	if req.Tradition != "" && req.Nakshatra != "" {
		var supported bool
		if err := a.db.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM traditions t JOIN cultures c ON c.id = t.culture_id
			              WHERE t.slug = $1 AND t.status = 'active' AND c.status = 'active')
			   AND EXISTS(SELECT 1 FROM nakshatras WHERE lower(canonical_name) = lower($2))`,
			req.Tradition, req.Nakshatra).Scan(&supported); err != nil {
			writeError(w, 503, "reviewed records unavailable")
			return
		}
		if !supported {
			writeError(w, 400, "unsupported tradition or Nakshatra")
			return
		}
		var matched bool
		if err := a.db.QueryRow(ctx, reviewedAssociationExistsSQL, req.Tradition, req.Nakshatra, req.Region).Scan(&matched); err != nil {
			writeError(w, 503, "reviewed records unavailable")
			return
		}
		if matched {
			association = assocReviewedMatch
		} else {
			association = assocNoReviewedMatch
		}
	}

	notOnRecord := []string{}
	if association != assocReviewedMatch {
		notOnRecord = append(notOnRecord,
			"a reviewed cultural association for this exact selection",
			"reviewed habitat or native range",
			"reviewed ecological role",
			"reviewed cultural or historical stories",
		)
	}

	envelope := a.runExplanation(r, task, ai.ExplainInput{
		Task:        task,
		Selection:   ai.SelectionFacts{Intention: label, Tradition: req.Tradition, Nakshatra: req.Nakshatra, Region: req.Region},
		Species:     aiSpeciesFacts(species),
		NotOnRecord: notOnRecord,
	})

	writeJSON(w, 200, map[string]any{
		"status": "ok",
		"task":   task,
		"evidence": evidenceStatus{
			CulturalAssociation: association,
			BotanicalIdentity:   botanicalIdentityStatus(species),
			Habitat:             "no_reviewed_record",
			EcologicalRole:      "no_reviewed_record",
			DocumentedStories:   "no_reviewed_record",
			PersonalMeaning:     "user_owned",
		},
		"trees":       species,
		"explanation": envelope,
	})
}

func botanicalIdentityStatus(species []treeFact) string {
	for _, s := range species {
		if s.Authority.TaxonKey != 0 && s.TaxonStatus == "ACCEPTED" {
			return "authority_verified"
		}
	}
	return "unavailable"
}

// runExplanation calls the provider and turns any failure into a precise,
// structured state. It never returns an error: an explanation is an extra, and
// the sourced botanical records are returned regardless.
func (a *API) runExplanation(r *http.Request, task string, in ai.ExplainInput) explanationEnvelope {
	if a.explainer == nil {
		return explanationEnvelope{Status: explainUnavailable, Reason: reasonNotConfigured}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	result, err := a.explainer.Explain(ctx, in)
	if err != nil {
		reason := classifyExplainError(err)
		a.log.Warn("explanation unavailable",
			"request_id", r.Header.Get("X-Request-ID"), "task", task, "reason", reason)
		return explanationEnvelope{Status: explainUnavailable, Reason: reason}
	}
	text := strings.TrimSpace(result.Text)
	if text == "" {
		return explanationEnvelope{Status: explainUnavailable, Reason: reasonEmpty}
	}
	return explanationEnvelope{Status: explainOK, Text: text, Model: result.Model}
}

func classifyExplainError(err error) string {
	switch {
	case errors.Is(err, ai.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return reasonTimeout
	case errors.Is(err, ai.ErrUnauthorized):
		return reasonUnauthorized
	case errors.Is(err, ai.ErrRateLimited):
		return reasonRateLimited
	case errors.Is(err, ai.ErrBadResponse), errors.Is(err, ai.ErrNoCompletion):
		return reasonEmpty
	case errors.Is(err, ai.ErrProvider):
		return reasonProvider
	default:
		return reasonProvider
	}
}

func aiSpeciesFacts(species []treeFact) []ai.SpeciesFacts {
	out := make([]ai.SpeciesFacts, 0, len(species))
	for _, s := range species {
		names := make([]string, 0, len(s.CommonNames))
		for _, n := range s.CommonNames {
			names = append(names, n.Name)
		}
		out = append(out, ai.SpeciesFacts{
			AcceptedName: s.AcceptedName,
			Authorship:   s.Authorship,
			Family:       s.Family,
			Genus:        s.Genus,
			Order:        s.Order,
			Rank:         s.Rank,
			TaxonStatus:  s.TaxonStatus,
			Protologue:   s.Protologue,
			CommonNames:  names,
			Authority:    fmt.Sprintf("%s (doi:%s)", s.Authority.Dataset, s.Authority.DOI),
		})
	}
	return out
}
