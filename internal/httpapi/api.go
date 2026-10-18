package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"everroot/internal/ai"
	"everroot/internal/ai/conversation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type questionAnswerer interface {
	AnswerQuestion(ctx context.Context, in ai.QuestionInput) (ai.AnswerResult, error)
}

type API struct {
	db        *pgxpool.Pool
	log       *slog.Logger
	mux       *http.ServeMux
	aiClient  *ai.Client
	qa        questionAnswerer
	convMgr   *conversation.Manager
	catalogue treeSource
	explainer explainer
}

func New(db *pgxpool.Pool, log *slog.Logger) (http.Handler, error) {
	aiClient, err := ai.New(log)
	if err != nil {
		log.Warn("ai layer is not configured", "reason", err)
		aiClient = nil
	}
	var cat treeSource
	if db != nil {
		cat = pgTreeSource{db: db}
	}
	a := &API{
		db:        db,
		log:       log,
		mux:       http.NewServeMux(),
		aiClient:  aiClient,
		convMgr:   conversation.New(db, aiClient, log),
		catalogue: cat,
	}
	// Only store the AI client in the interfaces when it really exists. Storing
	// a typed nil would make `a.qa == nil` false, so the "not configured" guard
	// in askQuestion would never fire and an unconfigured deployment would
	// report a misleading "temporarily unavailable" error instead.
	if aiClient != nil {
		a.qa = aiClient
		a.explainer = aiClient
	}
	a.routes()
	return a.requestID(a.cors(a.mux)), nil
}
func (a *API) routes() {
	a.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	a.mux.HandleFunc("GET /readyz", a.ready)
	a.mux.HandleFunc("GET /api/v1/traditions", a.traditions)
	a.mux.HandleFunc("GET /api/v1/traditions/{tradition}/regions", a.regions)
	a.mux.HandleFunc("GET /api/v1/nakshatras", a.nakshatras)
	a.mux.HandleFunc("GET /api/v1/trees", a.trees)
	a.mux.HandleFunc("POST /api/v1/discoveries", a.discover)
	a.mux.HandleFunc("POST /api/v1/explain", a.explain)
	a.mux.HandleFunc("POST /api/v1/questions", a.askQuestion)
	a.mux.HandleFunc("POST /api/v1/ask", a.askQuestion)
	a.mux.HandleFunc("POST /api/v1/feedback", a.submitFeedback)
	a.mux.HandleFunc("GET /api/v1/conversation", a.conversationGet)
	a.mux.HandleFunc("POST /api/v1/conversation", a.conversation)
	a.mux.Handle("GET /internal/v1/review/queue", a.admin(http.HandlerFunc(a.reviewQueue)))
	a.mux.Handle("POST /internal/v1/sources", a.admin(http.HandlerFunc(a.createSource)))
	a.mux.Handle("POST /internal/v1/sources/{sourceId}/evidence", a.admin(http.HandlerFunc(a.createEvidence)))
	a.mux.Handle("POST /internal/v1/sources/{sourceId}/review", a.admin(http.HandlerFunc(a.reviewSource)))
	a.mux.Handle("POST /internal/v1/mappings", a.admin(http.HandlerFunc(a.createMapping)))
	a.mux.Handle("POST /internal/v1/review/{revisionId}", a.admin(http.HandlerFunc(a.reviewRevision)))
	a.mux.Handle("POST /internal/v1/botanical-candidates/{candidateId}/review", a.admin(http.HandlerFunc(a.reviewBotanical)))
	a.mux.Handle("POST /internal/v1/publications/{revisionId}", a.admin(http.HandlerFunc(a.publication)))
}

// cors grants cross-origin access only when it is explicitly configured.
//
// The web app calls this API on its own origin — Vite proxies /api during
// development and vercel.json rewrites /api in production — so no grant is
// needed by default. Reflecting an arbitrary Origin whenever WEB_ORIGIN is
// unset would let any website proxy requests through this API, including the
// paid model calls behind /api/v1/questions.
func (a *API) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := strings.TrimSpace(os.Getenv("WEB_ORIGIN"))
		if origin != "" && allowed != "" && (allowed == "*" || allowed == origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, Accept")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *API) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("req-%d", time.Now().UnixNano())
		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, 503, "database is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.Ping(ctx); err != nil {
		writeError(w, 503, "database unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (a *API) admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("ADMIN_LOCAL_ENABLED") != "true" || !isLoopbackAddr(r.RemoteAddr) {
			writeError(w, 403, "local review access is disabled")
			return
		}
		role := os.Getenv("ADMIN_REVIEWER_ROLE")
		if role != "admin" && role != "cultural" && role != "regional_language" && role != "botanical" {
			writeError(w, 403, "reviewer role is not configured")
			return
		}
		if os.Getenv("ADMIN_REVIEWER_ID") == "" {
			writeError(w, 403, "reviewer identity is not configured")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (a *API) traditions(w http.ResponseWriter, r *http.Request) {
	fallback := []map[string]string{
		{"slug": "nakshatra-tree-association", "name": "Nakshatra Tree Association"},
	}
	if a.db == nil {
		writeJSON(w, 200, fallback)
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT t.slug,t.display_name FROM traditions t JOIN cultures c ON c.id=t.culture_id WHERE t.status='active' AND c.status='active' ORDER BY t.display_name`)
	if err != nil {
		writeJSON(w, 200, fallback)
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var slug, name string
		if err := rows.Scan(&slug, &name); err != nil {
			writeError(w, 500, "catalog read failed")
			return
		}
		out = append(out, map[string]string{"slug": slug, "name": name})
	}
	if rows.Err() != nil {
		writeJSON(w, 200, fallback)
		return
	}
	writeJSON(w, 200, out)
}
func (a *API) regions(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeJSON(w, 200, []string{})
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT tl.region_name FROM tradition_locales tl JOIN traditions t ON t.id=tl.tradition_id JOIN cultures c ON c.id=t.culture_id WHERE t.slug=$1 AND t.status='active' AND c.status='active' AND tl.status='active' ORDER BY tl.region_name`, r.PathValue("tradition"))
	if err != nil {
		writeJSON(w, 200, []string{})
		return
	}
	defer rows.Close()
	regions := []string{}
	for rows.Next() {
		var region string
		if err := rows.Scan(&region); err != nil {
			writeError(w, 500, "region catalog read failed")
			return
		}
		regions = append(regions, region)
	}
	if rows.Err() != nil {
		writeJSON(w, 200, []string{})
		return
	}
	writeJSON(w, 200, regions)
}

var canonicalNakshatras = []string{
	"Ashwini", "Bharani", "Krittika", "Rohini", "Mrigashira", "Ardra",
	"Punarvasu", "Pushya", "Ashlesha", "Magha", "Purva Phalguni", "Uttara Phalguni",
	"Hasta", "Chitra", "Swati", "Vishakha", "Anuradha", "Jyeshtha",
	"Mula", "Purva Ashadha", "Uttara Ashadha", "Shravana", "Dhanishta",
	"Shatabhisha", "Purva Bhadrapada", "Uttara Bhadrapada", "Revati",
}

func (a *API) nakshatras(w http.ResponseWriter, r *http.Request) {
	fallback := make([]map[string]any, len(canonicalNakshatras))
	for i, name := range canonicalNakshatras {
		fallback[i] = map[string]any{"name": name, "ordinal": i + 1}
	}
	if a.db == nil {
		writeJSON(w, 200, fallback)
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT canonical_name,ordinal FROM nakshatras ORDER BY ordinal`)
	if err != nil {
		writeJSON(w, 200, fallback)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var name string
		var ordinal int
		if err := rows.Scan(&name, &ordinal); err != nil {
			writeError(w, 500, "catalog read failed")
			return
		}
		out = append(out, map[string]any{"name": name, "ordinal": ordinal})
	}
	if rows.Err() != nil {
		writeJSON(w, 200, fallback)
		return
	}
	writeJSON(w, 200, out)
}

type discoveryRequest struct {
	Tradition string `json:"tradition"`
	Region    string `json:"region"`
	Nakshatra string `json:"nakshatra"`
}
type sourceResult struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Author             string     `json:"author"`
	Publisher          string     `json:"publisher"`
	PublicationDate    *time.Time `json:"publicationDate"`
	URL                string     `json:"url"`
	Bibliography       string     `json:"bibliographicReference"`
	Language           string     `json:"language"`
	AccessedOn         time.Time  `json:"accessedOn"`
	Attribution        string     `json:"attributionRequirements"`
	Rights             string     `json:"reuseStatus"`
	Locator            string     `json:"locator"`
	Text               *string    `json:"originalText"`
	Translation        *string    `json:"translation"`
	TranslationRights  string     `json:"translationRights"`
	VerificationStatus string     `json:"verificationStatus"`
	Relationship       string     `json:"relationship"`
}
type botanicalResult struct {
	CandidateID    string         `json:"candidateId"`
	ScientificName string         `json:"scientificName"`
	Certainty      string         `json:"certainty"`
	Confidence     float64        `json:"confidence"`
	ReviewStatus   string         `json:"reviewStatus"`
	Definitive     bool           `json:"definitive"`
	Evidence       []sourceResult `json:"evidence"`
}
type recommendation struct {
	MappingID           string            `json:"mappingId"`
	RevisionID          string            `json:"revisionId"`
	Version             int               `json:"version"`
	Tradition           string            `json:"tradition"`
	Region              string            `json:"region"`
	Nakshatra           string            `json:"nakshatra"`
	Claim               string            `json:"claim"`
	Interpretation      string            `json:"interpretation"`
	Sources             []sourceResult    `json:"sources"`
	BotanicalCandidates []botanicalResult `json:"botanicalCandidates"`
}

func (a *API) discover(w http.ResponseWriter, r *http.Request) {
	var req discoveryRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid JSON request")
		return
	}
	result, err := a.Discover(r.Context(), req.Tradition, req.Region, req.Nakshatra)
	if err != nil {
		var httpErr httpError
		if errors.As(err, &httpErr) {
			writeError(w, httpErr.code, httpErr.message)
			return
		}
		writeError(w, 503, "discovery unavailable")
		return
	}
	writeJSON(w, 200, result)
	return
}

// httpError carries an HTTP status code and message for typed errors.
type httpError struct {
	code    int
	message string
}

func (e httpError) Error() string {
	return e.message
}

// Discover performs the full discovery lookup: validates input, checks the
// tradition/Nakshatra/region, queries published mappings, loads source
// evidence and botanical candidates, and returns the response payload.
func (a *API) Discover(ctx context.Context, tradition, region, nakshatra string) (map[string]any, error) {
	tradition = strings.TrimSpace(tradition)
	region = strings.TrimSpace(region)
	nakshatra = strings.TrimSpace(nakshatra)
	if tradition == "" || nakshatra == "" || len(tradition) > 100 || len(region) > 100 || len(nakshatra) > 100 {
		return nil, httpError{code: 400, message: "tradition and supported Nakshatra are required"}
	}
	if a.db == nil {
		return map[string]any{
			"status":          "no_verified_mapping",
			"recommendations": []recommendation{},
		}, nil
	}
	var supported bool
	if err := a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM traditions t JOIN cultures c ON c.id=t.culture_id WHERE t.slug=$1 AND t.status='active' AND c.status='active') AND EXISTS(SELECT 1 FROM nakshatras WHERE lower(canonical_name)=lower($2))`, tradition, nakshatra).Scan(&supported); err != nil {
		a.log.Warn("database query failed during discovery; returning no_verified_mapping", "error", err)
		return map[string]any{
			"status":          "no_verified_mapping",
			"recommendations": []recommendation{},
		}, nil
	}
	if !supported {
		return nil, httpError{code: 400, message: "unsupported tradition or Nakshatra"}
	}
	if region != "" {
		var regionExists bool
		if err := a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tradition_locales tl JOIN traditions t ON t.id=tl.tradition_id WHERE t.slug=$1 AND tl.status='active' AND lower(tl.region_name)=lower($2))`, tradition, region).Scan(&regionExists); err != nil {
			return nil, httpError{code: 503, message: "discovery unavailable"}
		}
		if !regionExists {
			return nil, httpError{code: 400, message: "unsupported region for tradition"}
		}
	}
	rows, err := a.db.Query(ctx, `SELECT mr.id,mr.mapping_id,mr.version,t.display_name,COALESCE(tl.region_name,''),n.canonical_name,mr.original_claim,mr.interpretation
FROM mapping_revisions mr JOIN traditions t ON t.id=mr.tradition_id LEFT JOIN tradition_locales tl ON tl.id=mr.locale_id JOIN nakshatras n ON n.id=mr.nakshatra_id
JOIN LATERAL (SELECT action,revision_id FROM publication_events WHERE mapping_id=mr.mapping_id ORDER BY occurred_at DESC,id DESC LIMIT 1) pe ON pe.action='published' AND pe.revision_id=mr.id
WHERE t.slug=$1 AND lower(n.canonical_name)=lower($2) AND ($3='' OR lower(COALESCE(tl.region_name,''))=lower($3))
AND EXISTS(SELECT 1 FROM mapping_revision_evidence mre JOIN source_evidence se ON se.id=mre.evidence_id JOIN sources s ON s.id=se.source_id WHERE mre.revision_id=mr.id AND s.verification_status='verified' AND s.reuse_status IN ('permitted','citation_only'))
AND NOT EXISTS(SELECT 1 FROM mapping_revision_evidence mre JOIN source_evidence se ON se.id=mre.evidence_id JOIN sources s ON s.id=se.source_id WHERE mre.revision_id=mr.id AND (s.verification_status<>'verified' OR s.reuse_status NOT IN ('permitted','citation_only') OR (se.original_text IS NOT NULL AND s.reuse_status<>'permitted') OR (se.translation IS NOT NULL AND se.translation_rights<>'permitted')))
ORDER BY COALESCE(tl.region_name,''),mr.mapping_id,mr.version`, tradition, nakshatra, region)
	if err != nil {
		return nil, httpError{code: 503, message: "discovery unavailable"}
	}
	defer rows.Close()
	items := []recommendation{}
	for rows.Next() {
		var x recommendation
		if err := rows.Scan(&x.RevisionID, &x.MappingID, &x.Version, &x.Tradition, &x.Region, &x.Nakshatra, &x.Claim, &x.Interpretation); err != nil {
			return nil, httpError{code: 500, message: "discovery result invalid"}
		}
		items = append(items, x)
	}
	if rows.Err() != nil {
		return nil, httpError{code: 503, message: "discovery unavailable"}
	}
	for i := range items {
		items[i].Sources, err = a.loadSources(ctx, items[i].RevisionID)
		if err != nil {
			return nil, httpError{code: 503, message: "source evidence unavailable"}
		}
		items[i].BotanicalCandidates, err = a.loadCandidates(ctx, items[i].RevisionID)
		if err != nil {
			return nil, httpError{code: 503, message: "botanical information unavailable"}
		}
		approvedConfirmed := 0
		activeCandidates := 0
		for _, c := range items[i].BotanicalCandidates {
			if c.ReviewStatus != "rejected" {
				activeCandidates++
			}
			if c.ReviewStatus == "approved" && c.Certainty == "confirmed" {
				approvedConfirmed++
			}
		}
		if approvedConfirmed == 1 && activeCandidates == 1 && len(items[i].BotanicalCandidates) == 1 && len(items[i].BotanicalCandidates[0].Evidence) > 0 {
			for j := range items[i].BotanicalCandidates {
				c := &items[i].BotanicalCandidates[j]
				c.Definitive = c.ReviewStatus == "approved" && c.Certainty == "confirmed"
			}
		}
	}
	status := "verified_results"
	if len(items) == 0 {
		status = "no_verified_mapping"
	}
	return map[string]any{"status": status, "recommendations": items}, nil
}
func (a *API) loadSources(ctx context.Context, revisionID string) ([]sourceResult, error) {
	rows, err := a.db.Query(ctx, `SELECT se.id,s.title,s.author,s.publisher,s.publication_date,s.url,s.bibliographic_reference,s.language,s.accessed_on,s.attribution_requirements,s.reuse_status,se.locator,CASE WHEN s.reuse_status='permitted' THEN se.original_text ELSE NULL END,CASE WHEN se.translation_rights='permitted' THEN se.translation ELSE NULL END,se.translation_rights,s.verification_status,mre.relationship FROM mapping_revision_evidence mre JOIN source_evidence se ON se.id=mre.evidence_id JOIN sources s ON s.id=se.source_id WHERE mre.revision_id=$1 AND s.verification_status='verified' AND s.reuse_status IN ('permitted','citation_only') AND (se.original_text IS NULL OR s.reuse_status='permitted') AND (se.translation IS NULL OR se.translation_rights='permitted') ORDER BY s.title,se.locator`, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sourceResult{}
	for rows.Next() {
		var x sourceResult
		if err := rows.Scan(&x.ID, &x.Title, &x.Author, &x.Publisher, &x.PublicationDate, &x.URL, &x.Bibliography, &x.Language, &x.AccessedOn, &x.Attribution, &x.Rights, &x.Locator, &x.Text, &x.Translation, &x.TranslationRights, &x.VerificationStatus, &x.Relationship); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (a *API) loadCandidates(ctx context.Context, revisionID string) ([]botanicalResult, error) {
	rows, err := a.db.Query(ctx, `SELECT bc.id,ts.scientific_name,bc.certainty,bc.confidence,COALESCE((SELECT br.decision FROM botanical_reviews br WHERE br.candidate_id=bc.id ORDER BY br.reviewed_at DESC,br.id DESC LIMIT 1),'unreviewed') FROM botanical_candidates bc JOIN tree_species ts ON ts.id=bc.species_id WHERE bc.revision_id=$1 ORDER BY bc.id`, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []botanicalResult{}
	for rows.Next() {
		var x botanicalResult
		if err := rows.Scan(&x.CandidateID, &x.ScientificName, &x.Certainty, &x.Confidence, &x.ReviewStatus); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range out {
		out[i].Evidence, err = a.loadBotanicalEvidence(ctx, out[i].CandidateID)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (a *API) loadBotanicalEvidence(ctx context.Context, candidateID string) ([]sourceResult, error) {
	rows, err := a.db.Query(ctx, `SELECT se.id,s.title,s.author,s.publisher,s.publication_date,s.url,s.bibliographic_reference,s.language,s.accessed_on,s.attribution_requirements,s.reuse_status,se.locator,CASE WHEN s.reuse_status='permitted' THEN se.original_text ELSE NULL END,CASE WHEN se.translation_rights='permitted' THEN se.translation ELSE NULL END,se.translation_rights,s.verification_status,bce.relationship FROM botanical_candidate_evidence bce JOIN source_evidence se ON se.id=bce.evidence_id JOIN sources s ON s.id=se.source_id WHERE bce.candidate_id=$1 AND s.verification_status='verified' AND s.reuse_status IN ('permitted','citation_only') AND (se.original_text IS NULL OR s.reuse_status='permitted') AND (se.translation IS NULL OR se.translation_rights='permitted') ORDER BY s.title,se.locator`, candidateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sourceResult{}
	for rows.Next() {
		var x sourceResult
		if err := rows.Scan(&x.ID, &x.Title, &x.Author, &x.Publisher, &x.PublicationDate, &x.URL, &x.Bibliography, &x.Language, &x.AccessedOn, &x.Attribution, &x.Rights, &x.Locator, &x.Text, &x.Translation, &x.TranslationRights, &x.VerificationStatus, &x.Relationship); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func reviewerRole() string {
	role := os.Getenv("ADMIN_REVIEWER_ROLE")
	if role == "admin" {
		return "admin"
	}
	return role
}
func canReviewSource(role string) bool {
	return role == "admin" || role == "cultural" || role == "regional_language" || role == "botanical"
}
func audit(ctx context.Context, tx pgx.Tx, actor, action, entity string, id string, revisionID *string, requestID string, details any) error {
	b, _ := json.Marshal(details)
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,revision_id,request_id,details) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)`, actor, action, entity, id, revisionID, requestID, string(b))
	return err
}

func (a *API) createSource(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title           string `json:"title"`
		Type            string `json:"sourceType"`
		Author          string `json:"author"`
		Publisher       string `json:"publisher"`
		PublicationDate string `json:"publicationDate"`
		URL             string `json:"url"`
		Bibliography    string `json:"bibliographicReference"`
		Language        string `json:"language"`
		AccessedOn      string `json:"accessedOn"`
		Attribution     string `json:"attributionRequirements"`
		Rights          string `json:"reuseStatus"`
	}
	if decodeJSON(w, r, &in) != nil || strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Language) == "" || !oneOf(in.Rights, "permitted", "citation_only", "restricted", "unknown") {
		writeError(w, 400, "title, language, access date and valid reuse status are required")
		return
	}
	if in.URL == "" && strings.TrimSpace(in.Bibliography) == "" {
		writeError(w, 400, "a source URL or bibliographic reference is required")
		return
	}
	accessed, err := time.Parse("2006-01-02", in.AccessedOn)
	if err != nil {
		writeError(w, 400, "accessedOn must be YYYY-MM-DD")
		return
	}
	var publicationDate any
	if in.PublicationDate != "" {
		parsed, parseErr := time.Parse("2006-01-02", in.PublicationDate)
		if parseErr != nil {
			writeError(w, 400, "publicationDate must be YYYY-MM-DD")
			return
		}
		publicationDate = parsed
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, 503, "source store unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO sources(title,source_type,author,publisher,publication_date,url,bibliographic_reference,language,accessed_on,attribution_requirements,reuse_status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, in.Title, in.Type, in.Author, in.Publisher, publicationDate, in.URL, in.Bibliography, in.Language, accessed, in.Attribution, in.Rights).Scan(&id)
	if err == nil {
		err = audit(ctx, tx, os.Getenv("ADMIN_REVIEWER_ID"), "source_created", "source", id, nil, r.Header.Get("X-Request-ID"), map[string]string{"reuseStatus": in.Rights})
	}
	if err != nil {
		writeError(w, 400, "source could not be saved")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 500, "source could not be committed")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "verificationStatus": "unverified"})
}
func (a *API) createEvidence(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Locator           string  `json:"locator"`
		OriginalText      *string `json:"originalText"`
		Language          string  `json:"language"`
		Translation       *string `json:"translation"`
		TranslationRights string  `json:"translationRights"`
		Notes             string  `json:"evidenceNotes"`
	}
	if decodeJSON(w, r, &in) != nil || strings.TrimSpace(in.Locator) == "" || strings.TrimSpace(in.Language) == "" {
		writeError(w, 400, "locator and language are required")
		return
	}
	if in.TranslationRights == "" {
		in.TranslationRights = "not_applicable"
	}
	if !oneOf(in.TranslationRights, "permitted", "citation_only", "restricted", "unknown", "not_applicable") {
		writeError(w, 400, "invalid translation rights")
		return
	}
	var sourceRights string
	if err := a.db.QueryRow(r.Context(), `SELECT reuse_status FROM sources WHERE id=$1`, r.PathValue("sourceId")).Scan(&sourceRights); err != nil {
		writeError(w, 404, "source not found")
		return
	}
	if (in.OriginalText != nil || in.Translation != nil) && (sourceRights != "permitted" || in.Translation != nil && in.TranslationRights != "permitted") {
		writeError(w, 400, "passage text requires documented reuse permission")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, 503, "evidence store unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO source_evidence(source_id,locator,original_text,language,translation,translation_rights,evidence_notes) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, r.PathValue("sourceId"), in.Locator, in.OriginalText, in.Language, in.Translation, in.TranslationRights, in.Notes).Scan(&id)
	if err == nil {
		err = audit(ctx, tx, os.Getenv("ADMIN_REVIEWER_ID"), "evidence_created", "source_evidence", id, nil, r.Header.Get("X-Request-ID"), map[string]string{"sourceId": r.PathValue("sourceId")})
	}
	if err != nil {
		writeError(w, 400, "evidence could not be saved")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 500, "evidence could not be committed")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
func (a *API) reviewSource(w http.ResponseWriter, r *http.Request) {
	role := reviewerRole()
	if !canReviewSource(role) {
		writeError(w, 403, "reviewer role cannot review sources")
		return
	}
	var in struct {
		Decision string `json:"decision"`
		Notes    string `json:"notes"`
	}
	if decodeJSON(w, r, &in) != nil || !oneOf(in.Decision, "approved", "changes_requested", "rejected") {
		writeError(w, 400, "invalid source review decision")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, 503, "source review unavailable")
		return
	}
	defer tx.Rollback(ctx)
	id := r.PathValue("sourceId")
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sources WHERE id=$1)`, id).Scan(&exists); err != nil || !exists {
		writeError(w, 404, "source not found")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO source_reviews(source_id,reviewer_id,reviewer_role,decision,notes) VALUES($1,$2,$3,$4,$5)`, id, os.Getenv("ADMIN_REVIEWER_ID"), role, in.Decision, in.Notes)
	if err == nil {
		status := "unverified"
		if in.Decision == "approved" {
			status = "verified"
		}
		if in.Decision == "rejected" {
			status = "rejected"
		}
		_, err = tx.Exec(ctx, `UPDATE sources SET verification_status=$2 WHERE id=$1`, id, status)
		if err == nil && status != "verified" {
			rows, queryErr := tx.Query(ctx, `SELECT DISTINCT pm.mapping_id,pm.id FROM published_mapping_revisions pm JOIN mapping_revision_evidence mre ON mre.revision_id=pm.id JOIN source_evidence se ON se.id=mre.evidence_id WHERE se.source_id=$1`, id)
			if queryErr != nil {
				err = queryErr
			} else {
				type published struct{ mappingID, revisionID string }
				affected := []published{}
				for rows.Next() {
					var p published
					if scanErr := rows.Scan(&p.mappingID, &p.revisionID); scanErr != nil {
						err = scanErr
						break
					}
					affected = append(affected, p)
				}
				if err == nil {
					err = rows.Err()
				}
				rows.Close()
				for _, p := range affected {
					if err != nil {
						break
					}
					reason := "Source verification was revoked"
					_, err = tx.Exec(ctx, `INSERT INTO publication_events(mapping_id,revision_id,action,actor_id,reason) VALUES($1,$2,'withdrawn',$3,$4)`, p.mappingID, p.revisionID, os.Getenv("ADMIN_REVIEWER_ID"), reason)
					if err == nil {
						rev := p.revisionID
						err = audit(ctx, tx, os.Getenv("ADMIN_REVIEWER_ID"), "withdrawn", "mapping", p.mappingID, &rev, r.Header.Get("X-Request-ID"), map[string]string{"sourceId": id, "reason": reason})
					}
				}
			}
		}
	}
	if err == nil {
		err = audit(ctx, tx, os.Getenv("ADMIN_REVIEWER_ID"), "source_review", "source", id, nil, r.Header.Get("X-Request-ID"), map[string]string{"decision": in.Decision})
	}
	if err != nil {
		writeError(w, 500, "source review could not be recorded")
		return
	}
	if tx.Commit(ctx) != nil {
		writeError(w, 500, "source review could not be committed")
		return
	}
	w.WriteHeader(204)
}

type evidenceInput struct {
	ID           string `json:"id"`
	Relationship string `json:"relationship"`
}
type botanicalInput struct {
	ScientificName string          `json:"scientificName"`
	Certainty      string          `json:"certainty"`
	Confidence     float64         `json:"confidence"`
	Evidence       []evidenceInput `json:"evidence"`
}
type mappingInput struct {
	MappingID           string           `json:"mappingId"`
	Tradition           string           `json:"tradition"`
	RegionCode          string           `json:"regionCode"`
	Nakshatra           string           `json:"nakshatra"`
	Claim               string           `json:"claim"`
	Interpretation      string           `json:"interpretation"`
	Evidence            []evidenceInput  `json:"evidence"`
	BotanicalCandidates []botanicalInput `json:"botanicalCandidates"`
}

func (a *API) createMapping(w http.ResponseWriter, r *http.Request) {
	var in mappingInput
	if decodeJSON(w, r, &in) != nil || strings.TrimSpace(in.Tradition) == "" || strings.TrimSpace(in.Nakshatra) == "" || strings.TrimSpace(in.Claim) == "" || len(in.Evidence) == 0 {
		writeError(w, 400, "tradition, Nakshatra, original claim and evidence are required")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, 503, "mapping store unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var tid string
	if err = tx.QueryRow(ctx, `SELECT id FROM traditions WHERE slug=$1 AND status='active'`, in.Tradition).Scan(&tid); err != nil {
		writeError(w, 400, "unknown tradition")
		return
	}
	var localeID any
	if in.RegionCode != "" {
		var id string
		if err = tx.QueryRow(ctx, `SELECT id FROM tradition_locales WHERE tradition_id=$1 AND region_code=$2 AND status='active' LIMIT 1`, tid, in.RegionCode).Scan(&id); err != nil {
			writeError(w, 400, "unknown region for tradition")
			return
		}
		localeID = id
	}
	var nakID int16
	if err = tx.QueryRow(ctx, `SELECT id FROM nakshatras WHERE lower(canonical_name)=lower($1)`, in.Nakshatra).Scan(&nakID); err != nil {
		writeError(w, 400, "unsupported Nakshatra")
		return
	}
	var mappingID, revisionID string
	version := 1
	if in.MappingID == "" {
		err = tx.QueryRow(ctx, `INSERT INTO cultural_mappings DEFAULT VALUES RETURNING id`).Scan(&mappingID)
	} else {
		mappingID = in.MappingID
		err = tx.QueryRow(ctx, `SELECT id FROM cultural_mappings WHERE id=$1 FOR UPDATE`, mappingID).Scan(&mappingID)
		if err == nil {
			err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM mapping_revisions WHERE mapping_id=$1`, mappingID).Scan(&version)
		}
	}
	if err == nil {
		err = tx.QueryRow(ctx, `INSERT INTO mapping_revisions(mapping_id,version,tradition_id,locale_id,rule_type,nakshatra_id,original_claim,interpretation) VALUES($1,$2,$3,$4,'birth_nakshatra',$5,$6,$7) RETURNING id`, mappingID, version, tid, localeID, nakID, in.Claim, in.Interpretation).Scan(&revisionID)
	}
	if err != nil {
		writeError(w, 400, "mapping revision could not be created")
		return
	}
	for _, e := range in.Evidence {
		rel := e.Relationship
		if rel == "" {
			rel = "supports"
		}
		if !oneOf(rel, "supports", "qualifies", "contradicts", "context") {
			writeError(w, 400, "invalid evidence relationship")
			return
		}
		if _, err = tx.Exec(ctx, `INSERT INTO mapping_revision_evidence(revision_id,evidence_id,relationship) VALUES($1,$2,$3)`, revisionID, e.ID, rel); err != nil {
			writeError(w, 400, "evidence reference is invalid")
			return
		}
	}
	botanicalCandidateIDs := []string{}
	for _, bc := range in.BotanicalCandidates {
		if strings.TrimSpace(bc.ScientificName) == "" || !oneOf(bc.Certainty, "uncertain", "probable", "confirmed") || bc.Confidence < 0 || bc.Confidence > 1 || len(bc.Evidence) == 0 {
			writeError(w, 400, "each botanical candidate requires a scientific name, confidence, certainty and evidence")
			return
		}
		var speciesID, candidateID string
		err = tx.QueryRow(ctx, `INSERT INTO tree_species(scientific_name) VALUES($1) ON CONFLICT(scientific_name) DO NOTHING RETURNING id`, bc.ScientificName).Scan(&speciesID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `SELECT id FROM tree_species WHERE scientific_name=$1`, bc.ScientificName).Scan(&speciesID)
		}
		if err == nil {
			err = tx.QueryRow(ctx, `INSERT INTO botanical_candidates(revision_id,species_id,certainty,confidence) VALUES($1,$2,$3,$4) RETURNING id`, revisionID, speciesID, bc.Certainty, bc.Confidence).Scan(&candidateID)
		}
		if err != nil {
			writeError(w, 400, "botanical candidate could not be created")
			return
		}
		botanicalCandidateIDs = append(botanicalCandidateIDs, candidateID)
		for _, e := range bc.Evidence {
			rel := e.Relationship
			if rel == "" {
				rel = "supports"
			}
			if !oneOf(rel, "identifies", "supports", "qualifies", "contradicts") {
				writeError(w, 400, "invalid botanical evidence relationship")
				return
			}
			if _, err = tx.Exec(ctx, `INSERT INTO botanical_candidate_evidence(candidate_id,evidence_id,relationship) VALUES($1,$2,$3)`, candidateID, e.ID, rel); err != nil {
				writeError(w, 400, "botanical evidence reference is invalid")
				return
			}
		}
	}
	if err = audit(ctx, tx, os.Getenv("ADMIN_REVIEWER_ID"), "mapping_revision_created", "mapping_revision", revisionID, &revisionID, r.Header.Get("X-Request-ID"), map[string]int{"version": version}); err != nil {
		writeError(w, 500, "mapping audit failed")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 500, "mapping could not be committed")
		return
	}
	writeJSON(w, 201, map[string]any{"mappingId": mappingID, "revisionId": revisionID, "version": version, "botanicalCandidateIds": botanicalCandidateIDs})
}

func (a *API) reviewBotanical(w http.ResponseWriter, r *http.Request) {
	role := reviewerRole()
	if role != "botanical" && role != "admin" {
		writeError(w, 403, "botanical reviewer role is required")
		return
	}
	var in struct {
		Decision string `json:"decision"`
		Notes    string `json:"notes"`
	}
	if decodeJSON(w, r, &in) != nil || !oneOf(in.Decision, "approved", "changes_requested", "rejected") {
		writeError(w, 400, "invalid botanical review decision")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, 503, "botanical review unavailable")
		return
	}
	defer tx.Rollback(ctx)
	id := r.PathValue("candidateId")
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM botanical_candidates WHERE id=$1)`, id).Scan(&exists); err != nil || !exists {
		writeError(w, 404, "botanical candidate not found")
		return
	}
	if in.Decision == "approved" {
		var evidenceOK bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM botanical_candidate_evidence bce JOIN source_evidence se ON se.id=bce.evidence_id JOIN sources s ON s.id=se.source_id WHERE bce.candidate_id=$1 AND s.verification_status='verified' AND s.reuse_status IN ('permitted','citation_only') AND (se.original_text IS NULL OR s.reuse_status='permitted') AND (se.translation IS NULL OR se.translation_rights='permitted'))`, id).Scan(&evidenceOK)
		if err != nil || !evidenceOK {
			writeError(w, 400, "botanical approval requires verified evidence and documented rights")
			return
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO botanical_reviews(candidate_id,reviewer_id,reviewer_role,decision,notes) VALUES($1,$2,'botanical',$3,$4)`, id, os.Getenv("ADMIN_REVIEWER_ID"), in.Decision, in.Notes)
	if err == nil {
		err = audit(ctx, tx, os.Getenv("ADMIN_REVIEWER_ID"), "botanical_review", "botanical_candidate", id, nil, r.Header.Get("X-Request-ID"), map[string]string{"decision": in.Decision})
	}
	if err != nil {
		writeError(w, 500, "botanical review could not be recorded")
		return
	}
	if tx.Commit(ctx) != nil {
		writeError(w, 500, "botanical review could not be committed")
		return
	}
	w.WriteHeader(204)
}

func (a *API) reviewQueue(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT mr.id,mr.mapping_id,mr.version,t.display_name,COALESCE(tl.region_name,''),n.canonical_name,mr.original_claim,mr.interpretation,mr.created_at,COALESCE((SELECT decision FROM mapping_reviews WHERE revision_id=mr.id ORDER BY reviewed_at DESC,id DESC LIMIT 1),'pending'),COALESCE((SELECT CASE WHEN revision_id=mr.id THEN action ELSE 'unpublished' END FROM publication_events WHERE mapping_id=mr.mapping_id ORDER BY occurred_at DESC,id DESC LIMIT 1),'unpublished') FROM mapping_revisions mr JOIN traditions t ON t.id=mr.tradition_id LEFT JOIN tradition_locales tl ON tl.id=mr.locale_id JOIN nakshatras n ON n.id=mr.nakshatra_id WHERE mr.version=(SELECT MAX(newer.version) FROM mapping_revisions newer WHERE newer.mapping_id=mr.mapping_id) ORDER BY mr.created_at,mr.id`)
	if err != nil {
		writeError(w, 503, "review queue unavailable")
		return
	}
	defer rows.Close()
	type row struct {
		RevisionID          string            `json:"revisionId"`
		MappingID           string            `json:"mappingId"`
		Version             int               `json:"version"`
		Tradition           string            `json:"tradition"`
		Region              string            `json:"region"`
		Nakshatra           string            `json:"nakshatra"`
		Claim               string            `json:"claim"`
		Interpretation      string            `json:"interpretation"`
		CreatedAt           time.Time         `json:"createdAt"`
		LatestReview        string            `json:"latestReview"`
		PublicationState    string            `json:"publicationState"`
		Sources             []sourceResult    `json:"sources"`
		BotanicalCandidates []botanicalResult `json:"botanicalCandidates"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.RevisionID, &x.MappingID, &x.Version, &x.Tradition, &x.Region, &x.Nakshatra, &x.Claim, &x.Interpretation, &x.CreatedAt, &x.LatestReview, &x.PublicationState); err != nil {
			writeError(w, 500, "review queue invalid")
			return
		}
		out = append(out, x)
	}
	if rows.Err() != nil {
		writeError(w, 503, "review queue unavailable")
		return
	}
	rows.Close()
	for i := range out {
		out[i].Sources, err = a.loadRevisionEvidenceForReview(r.Context(), out[i].RevisionID)
		if err != nil {
			writeError(w, 503, "review evidence unavailable")
			return
		}
		out[i].BotanicalCandidates, err = a.loadCandidates(r.Context(), out[i].RevisionID)
		if err != nil {
			writeError(w, 503, "botanical review data unavailable")
			return
		}
	}
	writeJSON(w, 200, out)
}

func (a *API) loadRevisionEvidenceForReview(ctx context.Context, revisionID string) ([]sourceResult, error) {
	rows, err := a.db.Query(ctx, `SELECT se.id,s.title,s.author,s.publisher,s.publication_date,s.url,s.bibliographic_reference,s.language,s.accessed_on,s.attribution_requirements,s.reuse_status,se.locator,CASE WHEN s.reuse_status='permitted' THEN se.original_text ELSE NULL END,CASE WHEN se.translation_rights='permitted' THEN se.translation ELSE NULL END,se.translation_rights,s.verification_status,mre.relationship FROM mapping_revision_evidence mre JOIN source_evidence se ON se.id=mre.evidence_id JOIN sources s ON s.id=se.source_id WHERE mre.revision_id=$1 ORDER BY s.title,se.locator`, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sourceResult{}
	for rows.Next() {
		var x sourceResult
		if err := rows.Scan(&x.ID, &x.Title, &x.Author, &x.Publisher, &x.PublicationDate, &x.URL, &x.Bibliography, &x.Language, &x.AccessedOn, &x.Attribution, &x.Rights, &x.Locator, &x.Text, &x.Translation, &x.TranslationRights, &x.VerificationStatus, &x.Relationship); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (a *API) reviewRevision(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Decision string `json:"decision"`
		Notes    string `json:"notes"`
	}
	if err := decodeJSON(w, r, &req); err != nil || !oneOf(req.Decision, "approved", "changes_requested", "rejected") {
		writeError(w, 400, "invalid review decision")
		return
	}
	role := os.Getenv("ADMIN_REVIEWER_ROLE")
	if role == "botanical" {
		writeError(w, 403, "botanical reviewers cannot approve cultural mappings")
		return
	}
	id := r.PathValue("revisionId")
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, 503, "review unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var published bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM published_mapping_revisions WHERE id=$1)`, id).Scan(&published); err != nil {
		writeError(w, 503, "review unavailable")
		return
	}
	if published {
		writeError(w, 400, "published revisions cannot be re-reviewed; submit a new revision or withdraw it")
		return
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mapping_revisions WHERE id=$1)`, id).Scan(&exists); err != nil || !exists {
		writeError(w, 404, "revision not found")
		return
	}
	if role == "admin" {
		role = "cultural"
	}
	_, err = tx.Exec(ctx, `INSERT INTO mapping_reviews(revision_id,reviewer_id,reviewer_role,decision,notes) VALUES($1,$2,$3,$4,$5)`, id, os.Getenv("ADMIN_REVIEWER_ID"), role, req.Decision, req.Notes)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,revision_id,request_id,details) VALUES($1,'review','mapping_revision',$2,$2,$3,jsonb_build_object('decision',$4::text))`, os.Getenv("ADMIN_REVIEWER_ID"), id, r.Header.Get("X-Request-ID"), req.Decision)
	}
	if err != nil {
		a.log.Error("mapping review transaction failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
		writeError(w, 500, "review could not be recorded")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 500, "review could not be committed")
		return
	}
	w.WriteHeader(204)
}

func (a *API) publication(w http.ResponseWriter, r *http.Request) {
	role := reviewerRole()
	if role != "admin" && role != "cultural" && role != "regional_language" {
		writeError(w, 403, "cultural reviewer role is required for publication changes")
		return
	}
	var req struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(w, r, &req); err != nil || !oneOf(req.Action, "published", "unpublished", "withdrawn") || strings.TrimSpace(req.Reason) == "" {
		writeError(w, 400, "invalid publication action")
		return
	}
	id := r.PathValue("revisionId")
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, 503, "publication unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var mappingID string
	if err = tx.QueryRow(ctx, `SELECT mapping_id FROM mapping_revisions WHERE id=$1`, id).Scan(&mappingID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "revision not found")
		} else {
			writeError(w, 503, "publication unavailable")
		}
		return
	}
	if req.Action == "published" {
		var ok bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mapping_reviews r WHERE r.revision_id=$1 AND r.decision='approved' AND r.reviewer_role IN ('cultural','regional_language') AND NOT EXISTS(SELECT 1 FROM mapping_reviews newer WHERE newer.revision_id=r.revision_id AND (newer.reviewed_at,newer.id)>(r.reviewed_at,r.id))) AND EXISTS(SELECT 1 FROM mapping_revision_evidence mre JOIN source_evidence se ON se.id=mre.evidence_id JOIN sources s ON s.id=se.source_id WHERE mre.revision_id=$1 AND s.verification_status='verified' AND s.reuse_status IN ('permitted','citation_only'))`, id).Scan(&ok)
		if err != nil || !ok {
			writeError(w, 400, "publication gate failed")
			return
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO publication_events(mapping_id,revision_id,action,actor_id,reason) VALUES($1,$2,$3,$4,$5)`, mappingID, id, req.Action, os.Getenv("ADMIN_REVIEWER_ID"), req.Reason)
	if err != nil {
		writeError(w, 400, "publication gate failed")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity_type,entity_id,revision_id,request_id,details) VALUES($1,$2,'mapping',$3,$4,$5,jsonb_build_object('reason',$6::text))`, os.Getenv("ADMIN_REVIEWER_ID"), req.Action, mappingID, id, r.Header.Get("X-Request-ID"), req.Reason)
	if err != nil {
		writeError(w, 500, "publication audit failed")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 500, "publication could not be committed")
		return
	}
	w.WriteHeader(204)
}

func (a *API) conversation(w http.ResponseWriter, r *http.Request) {
	var req conversation.ConversationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid JSON request")
		return
	}
	ctx := r.Context()
	resp, err := a.convMgr.Handle(ctx, req)
	if err != nil {
		a.log.Error("conversation failed", "error", err)
		writeError(w, 503, "conversation unavailable")
		return
	}
	writeJSON(w, 200, resp)
}

func (a *API) conversationGet(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, 400, "session_id is required")
		return
	}
	ctx := r.Context()
	session, err := a.convMgr.GetSession(ctx, sessionID)
	if err != nil {
		a.log.Error("conversation get failed", "error", err)
		writeError(w, 404, "session not found")
		return
	}
	writeJSON(w, 200, session)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
func oneOf(v string, allowed ...string) bool {
	for _, x := range allowed {
		if v == x {
			return true
		}
	}
	return false
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
