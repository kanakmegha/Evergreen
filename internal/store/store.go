// Package store persists a visitor's own discoveries.
//
// It is the only package in the repository that writes to the user-data
// tables, and it holds no knowledge of trees, traditions or recommendations.
// Everything it saves arrived from a caller that had already generated an
// answer; nothing here decides what an answer is.
//
// Four properties hold throughout, and each is enforced by construction rather
// than by convention.
//
// Ownership is never taken from the request. Every read and every delete
// compares the stored owner_hash against the hash of the bearer token that was
// presented. A run whose id is correct but whose owner does not match is
// indistinguishable from a run that does not exist, so an id cannot become an
// existence oracle and guessing one buys nothing.
//
// A token is never stored. Only its SHA-256 is. The tokens are 32 bytes from
// crypto/rand, so there is no search space worth attacking; an unsalted hash
// is the right primitive for a high-entropy secret, and it means a database
// read never yields a usable credential.
//
// Retention has exactly one place to live. Every row is reachable from
// discovery_runs, which carries expires_at, so deleting a run cascades and no
// orphan can be left behind.
//
// Failure to persist is reported, never hidden. The caller decides whether a
// discovery is still worth returning when its save failed, but it decides that
// knowing, which is why nothing here swallows an error.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors the store reports. They are deliberately coarse: nothing here tells a
// caller whether a run id exists, only whether the caller may see it.
var (
	// ErrUnauthorized means the presented token is unknown, expired or
	// revoked.
	ErrUnauthorized = errors.New("the session token is not valid")
	// ErrForbidden means the caller may not read the named run. It is returned
	// for both "not yours" and "does not exist", so the two cannot be told
	// apart.
	ErrForbidden = errors.New("that discovery is not available")
	// ErrUnavailable means the database could not be reached. A caller that
	// receives this knows the answer it already has is fine and only the save
	// failed.
	ErrUnavailable = errors.New("saving is temporarily unavailable")
)

// tokenBytes is the entropy in an issued bearer token.
const tokenBytes = 32

// tokenPrefix namespaces issued tokens, so a leaked string that is not one of
// Everroot's can be recognised as such at a glance rather than being hashed
// and probed.
const tokenPrefix = "evr_"

// Owner is the caller, reduced to what authorisation needs.
//
// TokenHash is the only thing the store will compare against. UserID is the
// linked account and is empty for an anonymous visitor, which is the normal
// case rather than a degraded one.
type Owner struct {
	TokenHash []byte
	UserID    string
}

// Store reads and writes the user-data tables.
type Store struct {
	db  *pgxpool.Pool
	log *slog.Logger
}

// New builds a store. A nil pool makes every call report ErrUnavailable
// rather than panicking, because a deployment with no database still has to
// serve discoveries — it just cannot save them, and it has to say so.
func New(db *pgxpool.Pool, log *slog.Logger) *Store {
	return &Store{db: db, log: log}
}

// ready reports whether the store can be used at all.
func (s *Store) ready() error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return nil
}

// HashToken reduces a bearer token to its stored form.
//
// A token that does not carry Everroot's prefix is hashed anyway rather than
// rejected early: rejecting on shape would leak which strings are tokens.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return sum[:]
}

// NewAnonymousToken issues a token for a visitor who has not signed in.
//
// The plaintext is returned exactly once and is never recoverable afterwards —
// not by the store, not from the database. That is the point: an anonymous
// discovery has to be readable again later without Everroot knowing who made
// it, and a stored plaintext would make the table a credential store.
func (s *Store) NewAnonymousToken(ctx context.Context) (string, Owner, error) {
	if err := s.ready(); err != nil {
		return "", Owner{}, err
	}

	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		// A CSPRNG failure is not something to paper over: issuing a
		// predictable token would be far worse than issuing none.
		return "", Owner{}, fmt.Errorf("generate token: %w", err)
	}
	token := tokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	hash := HashToken(token)

	var id string
	err := s.db.QueryRow(ctx,
		`INSERT INTO auth_tokens (token_hash, kind) VALUES ($1, 'anonymous') RETURNING id`,
		hash).Scan(&id)
	if err != nil {
		return "", Owner{}, s.unavailable(err)
	}
	return token, Owner{TokenHash: hash}, nil
}

// TokenExpiry reads back when a freshly issued token lapses.
//
// It exists so the session response can tell a client when to come back for a
// new token. Reading it from the database rather than recomputing it here
// means the date the visitor is shown is the date the database will actually
// enforce.
func (s *Store) TokenExpiry(ctx context.Context, token string) (string, error) {
	if err := s.ready(); err != nil {
		return "", err
	}
	var expiresAt time.Time
	err := s.db.QueryRow(ctx,
		`SELECT expires_at FROM auth_tokens WHERE token_hash = $1`,
		HashToken(token)).Scan(&expiresAt)
	if err != nil {
		return "", s.unavailable(err)
	}
	return expiresAt.UTC().Format(time.RFC3339), nil
}

// Authorize resolves a bearer token to its owner.
//
// A token that is unknown, expired or revoked all report ErrUnauthorized. The
// three are not told apart because telling them apart tells an attacker which
// of their guesses were once real.
func (s *Store) Authorize(ctx context.Context, token string) (Owner, error) {
	if err := s.ready(); err != nil {
		return Owner{}, err
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return Owner{}, ErrUnauthorized
	}
	hash := HashToken(trimmed)

	var userID *string
	var expiresAt time.Time
	var revokedAt *time.Time
	err := s.db.QueryRow(ctx,
		`SELECT user_id, expires_at, revoked_at
		   FROM auth_tokens
		  WHERE token_hash = $1`,
		hash).Scan(&userID, &expiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Owner{}, ErrUnauthorized
	}
	if err != nil {
		return Owner{}, s.unavailable(err)
	}
	if revokedAt != nil || !expiresAt.After(time.Now()) {
		return Owner{}, ErrUnauthorized
	}

	owner := Owner{TokenHash: hash}
	if userID != nil {
		owner.UserID = *userID
	}

	// last_used_at is an operational fact about a token, not about a
	// visitor's answers, so it is safe to record. A failure here must not
	// cost the caller their request.
	if _, err := s.db.Exec(ctx,
		`UPDATE auth_tokens SET last_used_at = now() WHERE token_hash = $1`, hash); err != nil {
		s.logger().Warn("could not record token use", "error", err)
	}
	return owner, nil
}

// SavedRun is one discovery, as the caller wants it stored.
type SavedRun struct {
	Intention      string
	Description    string
	Tradition      string
	Region         string
	Nakshatra      string
	Constraints    []string
	ResearchStatus string
	PromptVersion  string
	Model          string
	Disclaimer     string
	// Payload is the structured result, stored verbatim as JSONB.
	Payload any
	// Sources are the retrieved findings. They are saved with the result and
	// never into a shared table: research retrieved for one visitor is not a
	// global fact about a tree.
	Sources []SavedSource
	// FollowUp is the question the model asked, if any.
	FollowUp string
	// Failed is set when the discovery did not complete. A failed run is
	// still worth recording, because a visitor's own question is worth having
	// and the failure category is credential-free.
	Failed        bool
	ErrorCategory string
}

// SavedSource is one retrieved finding attached to a stored result.
type SavedSource struct {
	SourceID    string
	Title       string
	URL         string
	Publisher   string
	Kind        string
	Snippet     string
	RetrievedAt time.Time
}

// Save persists one discovery together with its sources.
//
// It runs in a single transaction so a result can never be stored without the
// sources its citations point at — a card whose citation list is half missing
// would be worse than no card at all.
//
// The owner comes from the Owner argument, never from the payload. Nothing a
// caller sends can make a run readable by someone else.
func (s *Store) Save(ctx context.Context, owner Owner, run SavedRun) (string, error) {
	if err := s.ready(); err != nil {
		return "", err
	}
	if len(owner.TokenHash) == 0 {
		return "", ErrUnauthorized
	}

	payload, err := json.Marshal(run.Payload)
	if err != nil {
		return "", fmt.Errorf("encode result payload: %w", err)
	}
	constraints, err := json.Marshal(orEmpty(run.Constraints))
	if err != nil {
		return "", fmt.Errorf("encode constraints: %w", err)
	}

	status := "complete"
	if run.Failed {
		status = "failed"
	}
	researchStatus := normaliseResearchStatus(run.ResearchStatus)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", s.unavailable(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID *string
	if owner.UserID != "" {
		userID = &owner.UserID
	}

	var runID string
	err = tx.QueryRow(ctx, `
		INSERT INTO discovery_runs
			(owner_hash, user_id, intention, description, tradition, region, nakshatra,
			 constraints, research_status, status, error_category, completed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, now())
		RETURNING id`,
		owner.TokenHash, userID,
		run.Intention, nullable(run.Description), nullable(run.Tradition),
		nullable(run.Region), nullable(run.Nakshatra),
		constraints, researchStatus, status, nullable(run.ErrorCategory),
	).Scan(&runID)
	if err != nil {
		return "", s.unavailable(err)
	}

	// The visitor's own words and the follow-up question, in order. These are
	// the two pieces of text a visitor would want to see again, and nothing
	// else from the exchange is worth keeping.
	messages := []struct {
		role    string
		content string
		seq     int
	}{
		{"user", firstOf(run.Description, run.Intention), 0},
	}
	if strings.TrimSpace(run.FollowUp) != "" {
		messages = append(messages, struct {
			role    string
			content string
			seq     int
		}{"follow_up", run.FollowUp, 1})
	}
	for _, m := range messages {
		if strings.TrimSpace(m.content) == "" {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO discovery_messages (run_id, role, content, seq) VALUES ($1,$2,$3,$4)`,
			runID, m.role, m.content, m.seq); err != nil {
			return "", s.unavailable(err)
		}
	}

	var resultID string
	err = tx.QueryRow(ctx, `
		INSERT INTO discovery_results
			(run_id, payload, prompt_version, model, research_status, disclaimer)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id`,
		runID, payload, orDefault(run.PromptVersion, "unknown"),
		nullable(run.Model), researchStatus, nullable(run.Disclaimer),
	).Scan(&resultID)
	if err != nil {
		return "", s.unavailable(err)
	}

	for _, src := range run.Sources {
		if strings.TrimSpace(src.URL) == "" || strings.TrimSpace(src.Title) == "" {
			// A source without an address or a title is not something a
			// visitor can follow, so it is not stored as one.
			continue
		}
		retrieved := src.RetrievedAt
		if retrieved.IsZero() {
			retrieved = time.Now().UTC()
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO discovery_sources
				(result_id, source_id, title, url, publisher, kind, snippet, retrieved_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (result_id, source_id) DO NOTHING`,
			resultID, src.SourceID, src.Title, src.URL,
			nullable(src.Publisher), orDefault(src.Kind, "web_page"),
			nullable(src.Snippet), retrieved,
		); err != nil {
			return "", s.unavailable(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", s.unavailable(err)
	}
	return runID, nil
}

// RunSummary is one saved discovery as the list renders it.
type RunSummary struct {
	ID             string    `json:"id"`
	Intention      string    `json:"intention"`
	ResearchStatus string    `json:"researchStatus"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	// SourceCount is how many findings the result carried, so a list can show
	// a researched answer apart from a provisional one without loading every
	// source.
	SourceCount int `json:"sourceCount"`
}

// Run is one saved discovery with its result and sources.
type Run struct {
	RunSummary
	Description   string          `json:"description,omitempty"`
	Tradition     string          `json:"tradition,omitempty"`
	Region        string          `json:"region,omitempty"`
	Nakshatra     string          `json:"nakshatra,omitempty"`
	FollowUp      string          `json:"followUpQuestion,omitempty"`
	Disclaimer    string          `json:"disclaimer,omitempty"`
	PromptVersion string          `json:"promptVersion,omitempty"`
	Model         string          `json:"model,omitempty"`
	Payload       json.RawMessage `json:"payload"`
	Sources       []SavedSource   `json:"sources"`
}

// List returns the caller's own discoveries, newest first.
//
// It reads only rows whose owner_hash matches. There is no administration
// path here and no id-based override: the caller sees what they created and
// nothing else.
func (s *Store) List(ctx context.Context, owner Owner, limit int) ([]RunSummary, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if len(owner.TokenHash) == 0 {
		return nil, ErrUnauthorized
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	rows, err := s.db.Query(ctx, `
		SELECT r.id, r.intention, r.research_status, r.status, r.created_at,
		       (SELECT count(*) FROM discovery_sources ds
		         JOIN discovery_results dr ON dr.id = ds.result_id
		        WHERE dr.run_id = r.id)
		  FROM discovery_runs r
		 WHERE r.owner_hash = $1
		 ORDER BY r.created_at DESC
		 LIMIT $2`,
		owner.TokenHash, limit)
	if err != nil {
		return nil, s.unavailable(err)
	}
	defer rows.Close()

	out := make([]RunSummary, 0, limit)
	for rows.Next() {
		var row RunSummary
		if err := rows.Scan(&row.ID, &row.Intention, &row.ResearchStatus, &row.Status, &row.CreatedAt, &row.SourceCount); err != nil {
			return nil, s.unavailable(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, s.unavailable(err)
	}
	return out, nil
}

// Get returns one of the caller's discoveries.
//
// The WHERE clause carries both the id and the owner. A run belonging to
// someone else is reported exactly like one that does not exist, so the
// endpoint cannot be used to discover which ids are real.
func (s *Store) Get(ctx context.Context, owner Owner, id string) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	if len(owner.TokenHash) == 0 {
		return Run{}, ErrUnauthorized
	}

	var run Run
	var description, tradition, region, nakshatra, followUp, disclaimer *string
	var promptVersion, model *string
	err := s.db.QueryRow(ctx, `
		SELECT r.id, r.intention, r.description, r.tradition, r.region, r.nakshatra,
		       r.research_status, r.status, r.created_at,
		       m.content, res.disclaimer, res.prompt_version, res.model, res.payload
		  FROM discovery_runs r
		  LEFT JOIN discovery_messages m
		         ON m.run_id = r.id AND m.role = 'follow_up'
		  LEFT JOIN discovery_results res ON res.run_id = r.id
		 WHERE r.id = $1 AND r.owner_hash = $2`,
		id, owner.TokenHash).Scan(
		&run.ID, &run.Intention, &description, &tradition, &region, &nakshatra,
		&run.ResearchStatus, &run.Status, &run.CreatedAt,
		&followUp, &disclaimer, &promptVersion, &model, &run.Payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrForbidden
	}
	if err != nil {
		return Run{}, s.unavailable(err)
	}

	run.Description = deref(description)
	run.Tradition = deref(tradition)
	run.Region = deref(region)
	run.Nakshatra = deref(nakshatra)
	run.FollowUp = deref(followUp)
	run.Disclaimer = deref(disclaimer)
	run.PromptVersion = deref(promptVersion)
	run.Model = deref(model)

	sources, err := s.loadSources(ctx, id)
	if err != nil {
		return Run{}, err
	}
	run.Sources = sources
	run.SourceCount = len(sources)
	return run, nil
}

// loadSources reads the findings saved with one run. The ownership check
// happened on the run itself, so a source row is only ever reached through a
// run the caller already proved they own.
func (s *Store) loadSources(ctx context.Context, runID string) ([]SavedSource, error) {
	rows, err := s.db.Query(ctx, `
		SELECT ds.source_id, ds.title, ds.url, ds.publisher, ds.kind, ds.snippet, ds.retrieved_at
		  FROM discovery_sources ds
		  JOIN discovery_results dr ON dr.id = ds.result_id
		 WHERE dr.run_id = $1
		 ORDER BY ds.source_id`,
		runID)
	if err != nil {
		return nil, s.unavailable(err)
	}
	defer rows.Close()

	out := []SavedSource{}
	for rows.Next() {
		var src SavedSource
		var publisher, snippet *string
		if err := rows.Scan(&src.SourceID, &src.Title, &src.URL, &publisher, &src.Kind, &snippet, &src.RetrievedAt); err != nil {
			return nil, s.unavailable(err)
		}
		src.Publisher = deref(publisher)
		src.Snippet = deref(snippet)
		out = append(out, src)
	}
	if err := rows.Err(); err != nil {
		return nil, s.unavailable(err)
	}
	return out, nil
}

// Delete removes one of the caller's discoveries.
//
// It matches on id AND owner for the same reason Get does. A delete that
// matched zero rows is reported as ErrForbidden rather than as success, so a
// caller cannot be told they deleted something they never owned.
func (s *Store) Delete(ctx context.Context, owner Owner, id string) error {
	if err := s.ready(); err != nil {
		return err
	}
	if len(owner.TokenHash) == 0 {
		return ErrUnauthorized
	}

	tag, err := s.db.Exec(ctx,
		`DELETE FROM discovery_runs WHERE id = $1 AND owner_hash = $2`,
		id, owner.TokenHash)
	if err != nil {
		return s.unavailable(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

// CleanupExpired applies the retention rule and reports how many runs it
// removed. Every message, result and source cascades from the run, so this one
// statement is the whole rule.
func (s *Store) CleanupExpired(ctx context.Context) (int64, error) {
	if err := s.ready(); err != nil {
		return 0, err
	}
	var removed int64
	if err := s.db.QueryRow(ctx, `SELECT cleanup_expired_discoveries()`).Scan(&removed); err != nil {
		return 0, s.unavailable(err)
	}
	return removed, nil
}

func (s *Store) logger() *slog.Logger {
	if s != nil && s.log != nil {
		return s.log
	}
	return slog.Default()
}

// unavailable maps a driver error onto the sentinel without carrying its text
// outward, because a pgx error can embed the host and the user from the DSN.
// The full error goes to the log, where it is safe and where it is needed.
func (s *Store) unavailable(err error) error {
	s.logger().Warn("user data store unavailable", "error", err)
	return ErrUnavailable
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// normaliseResearchStatus keeps the stored value inside the column's CHECK
// constraint, so an unexpected status can never fail a save.
func normaliseResearchStatus(value string) string {
	switch value {
	case "researched", "partial":
		return value
	}
	return "unavailable"
}
