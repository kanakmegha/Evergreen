package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to a scratch database when one is configured, and returns
// nil otherwise.
//
// The integration tests below are gated on EVERROOT_TEST_DATABASE_URL so the
// regular suite stays green and offline. They are the only tests in this
// repository that can prove an ownership rule, because an ownership rule is a
// property of the SQL and not of the Go: a unit test with a fake pool would
// only prove that the fake behaved.
//
// Use a throwaway database. These tests create and delete rows.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("EVERROOT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set EVERROOT_TEST_DATABASE_URL to a scratch database to run the store integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	pool := testPool(t)
	// Every test owns its rows by token, so a failure to clean up costs
	// nothing but a little space in a database nobody keeps.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `DELETE FROM auth_tokens WHERE token_hash NOT IN (SELECT token_hash FROM auth_tokens LIMIT 0)`)
		_, _ = pool.Exec(ctx, `DELETE FROM discovery_runs`)
		_, _ = pool.Exec(ctx, `DELETE FROM auth_tokens`)
	})
	return New(pool, nil)
}

func TestHashTokenIsStableAndScoped(t *testing.T) {
	a := HashToken("evr_abc")
	b := HashToken("evr_abc")
	if string(a) != string(b) {
		t.Fatal("the same token must hash identically, or a session could never be found again")
	}
	if string(a) == string(HashToken("evr_abd")) {
		t.Fatal("different tokens must hash differently")
	}
	// Whitespace is a transport artefact, not part of the secret.
	if string(a) != string(HashToken("  evr_abc  ")) {
		t.Fatal("surrounding whitespace must not change the hash, or a copied token would stop working")
	}
	if len(a) != sha256.Size {
		t.Fatalf("hash is %d bytes, want a full SHA-256", len(a))
	}
}

// TestUnconfiguredStoreFailsHonestly: a deployment with no database must
// report that saving is unavailable rather than pretending a save succeeded —
// and equally must not tell a visitor their token is invalid when the truth is
// that nobody could be asked.
func TestUnconfiguredStoreFailsHonestly(t *testing.T) {
	var s *Store // nil on purpose
	ctx := context.Background()

	if _, _, err := s.NewAnonymousToken(ctx); !errors.Is(err, ErrUnavailable) {
		t.Errorf("NewAnonymousToken = %v, want ErrUnavailable", err)
	}
	if _, err := s.Authorize(ctx, "evr_anything"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Authorize = %v, want ErrUnavailable; a database outage must not read as a bad token", err)
	}
	// A caller with no token at all is refused. With no database there is
	// nothing to authenticate against, so the honest answer is that storage is
	// unavailable rather than that the caller did something wrong.
	if _, err := s.List(ctx, Owner{}, 10); !errors.Is(err, ErrUnavailable) {
		t.Errorf("List with no owner = %v, want ErrUnavailable", err)
	}
	if _, err := s.Get(ctx, Owner{}, "x"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Get with no owner = %v, want ErrUnavailable", err)
	}
	if err := s.Delete(ctx, Owner{}, "x"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Delete with no owner = %v, want ErrUnavailable", err)
	}
	// A caller WITH a token, against a database that is not there, gets the
	// same verdict: the difference only matters once there is a database to
	// ask, which is covered by the ownership integration test below.
	owned := Owner{TokenHash: []byte("hash")}
	if _, err := s.List(ctx, owned, 10); !errors.Is(err, ErrUnavailable) {
		t.Errorf("List = %v, want ErrUnavailable", err)
	}
	if _, err := s.Get(ctx, owned, "x"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Get = %v, want ErrUnavailable", err)
	}

	empty := New(nil, nil)
	if _, err := empty.Save(ctx, Owner{TokenHash: []byte("x")}, SavedRun{Intention: "i"}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Save = %v, want ErrUnavailable", err)
	}
}

func TestNormaliseResearchStatusStaysInsideTheCheckConstraint(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"researched", "researched"},
		{"partial", "partial"},
		{"", "unavailable"},
		{"RESEARCHED", "unavailable"}, // never trust a caller's casing against a CHECK
		{"something else", "unavailable"},
	} {
		if got := normaliseResearchStatus(tc.in); got != tc.want {
			t.Errorf("normaliseResearchStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNullableAndDerefRoundTrip(t *testing.T) {
	if nullable("  ") != nil {
		t.Error("a blank string must become SQL NULL, or a CHECK or NOT NULL could reject a save")
	}
	if nullable("x") != "x" {
		t.Error("a real value must survive")
	}
	if deref(nil) != "" {
		t.Error("deref(nil) must be empty rather than panic")
	}
	unchanged := "kept"
	if deref(&unchanged) != "kept" {
		t.Error("deref must return the value")
	}
}

// ---------------------------------------------------------------------------
// Integration tests. These are the ones that can prove an ownership rule.
// ---------------------------------------------------------------------------

func TestTokenLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	token, owner, err := s.NewAnonymousToken(ctx)
	if err != nil {
		t.Fatalf("NewAnonymousToken: %v", err)
	}
	if len(owner.TokenHash) == 0 {
		t.Fatal("an issued token must come back with its owner hash")
	}

	// The plaintext must never be recoverable from the database. Reading it
	// back and comparing is the only way to be sure.
	var stored []byte
	if err := s.db.QueryRow(ctx,
		`SELECT token_hash FROM auth_tokens WHERE token_hash = $1`, owner.TokenHash).Scan(&stored); err != nil {
		t.Fatalf("the issued token was not stored: %v", err)
	}
	if string(stored) == token {
		t.Fatal("the plaintext token was stored; only its hash may be")
	}

	authorized, err := s.Authorize(ctx, token)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if string(authorized.TokenHash) != string(owner.TokenHash) {
		t.Error("Authorize returned a different owner than the one issued")
	}

	if _, err := s.Authorize(ctx, "evr_not-a-real-token"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("an unknown token = %v, want ErrUnauthorized", err)
	}
	if _, err := s.Authorize(ctx, ""); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("an empty token = %v, want ErrUnauthorized", err)
	}

	expiry, err := s.TokenExpiry(ctx, token)
	if err != nil {
		t.Fatalf("TokenExpiry: %v", err)
	}
	if expiry == "" {
		t.Error("a client must be told when its session lapses")
	}
}

func TestOwnershipPreventsCrossUserAccess(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tokenA, ownerA, err := s.NewAnonymousToken(ctx)
	if err != nil {
		t.Fatalf("token A: %v", err)
	}
	_ = tokenA
	_, ownerB, err := s.NewAnonymousToken(ctx)
	if err != nil {
		t.Fatalf("token B: %v", err)
	}

	runID, err := s.Save(ctx, ownerA, SavedRun{
		Intention:      "a tree for my mother",
		Description:    "something she would have recognised",
		ResearchStatus: "researched",
		PromptVersion:  "v1.0-research-grounded",
		Payload:        map[string]any{"candidates": []any{}},
		Sources: []SavedSource{{
			SourceID: "s1", Title: "Ashoka",
			URL:  "https://en.wikipedia.org/wiki/Ashoka_tree",
			Kind: "encyclopedia",
		}},
		FollowUp: "Which climate is she in?",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The owner can read it back, with its sources and its follow-up question.
	run, err := s.Get(ctx, ownerA, runID)
	if err != nil {
		t.Fatalf("Get by owner: %v", err)
	}
	if run.Intention != "a tree for my mother" {
		t.Errorf("intention = %q", run.Intention)
	}
	if run.FollowUp != "Which climate is she in?" {
		t.Errorf("followUp = %q; a saved discovery must keep the question that was asked", run.FollowUp)
	}
	if len(run.Sources) != 1 || run.Sources[0].URL != "https://en.wikipedia.org/wiki/Ashoka_tree" {
		t.Errorf("sources = %+v", run.Sources)
	}

	// Another visitor must not be able to read it, delete it, or even learn
	// that it exists. Both a wrong id and a right id with the wrong owner
	// report the same error, so an id cannot be probed.
	if _, err := s.Get(ctx, ownerB, runID); !errors.Is(err, ErrForbidden) {
		t.Errorf("another visitor's Get = %v, want ErrForbidden", err)
	}
	if err := s.Delete(ctx, ownerB, runID); !errors.Is(err, ErrForbidden) {
		t.Errorf("another visitor's Delete = %v, want ErrForbidden", err)
	}
	if _, err := s.Get(ctx, ownerB, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrForbidden) {
		t.Errorf("a nonexistent id = %v, want ErrForbidden — identical to a foreign run", err)
	}

	// The other visitor's list is empty, and the owner's is not.
	theirs, err := s.List(ctx, ownerB, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(theirs) != 0 {
		t.Errorf("another visitor sees %d runs, want 0", len(theirs))
	}
	ours, err := s.List(ctx, ownerA, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ours) != 1 {
		t.Errorf("the owner sees %d runs, want 1", len(ours))
	}
	if ours[0].SourceCount != 1 {
		t.Errorf("sourceCount = %d, want the saved source so a list can show research", ours[0].SourceCount)
	}

	// The owner can delete it, and afterwards it is gone for them too.
	if err := s.Delete(ctx, ownerA, runID); err != nil {
		t.Fatalf("Delete by owner: %v", err)
	}
	if _, err := s.Get(ctx, ownerA, runID); !errors.Is(err, ErrForbidden) {
		t.Errorf("after deletion, Get = %v, want ErrForbidden", err)
	}
}

func TestDeleteCascadesToEveryChildRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, owner, err := s.NewAnonymousToken(ctx)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	runID, err := s.Save(ctx, owner, SavedRun{
		Intention:      "curiosity",
		ResearchStatus: "researched",
		PromptVersion:  "v1",
		Payload:        map[string]any{"candidates": []any{}},
		Sources: []SavedSource{
			{SourceID: "s1", Title: "One", URL: "https://example.org/1", Kind: "web_page"},
			{SourceID: "s2", Title: "Two", URL: "https://example.org/2", Kind: "web_page"},
		},
		FollowUp: "Where would it be planted?",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	var before int
	if err := s.db.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM discovery_messages WHERE run_id = $1)
		     + (SELECT count(*) FROM discovery_sources s
		          JOIN discovery_results r ON r.id = s.result_id WHERE r.run_id = $1)`,
		runID).Scan(&before); err != nil {
		t.Fatalf("count children: %v", err)
	}
	if before < 3 {
		t.Fatalf("expected a follow-up message and two sources, found %d child rows", before)
	}

	if err := s.Delete(ctx, owner, runID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var after int
	if err := s.db.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM discovery_messages WHERE run_id = $1)
		     + (SELECT count(*) FROM discovery_results WHERE run_id = $1)
		     + (SELECT count(*) FROM discovery_sources s
		          JOIN discovery_results r ON r.id = s.result_id WHERE r.run_id = $1)`,
		runID).Scan(&after); err != nil {
		t.Fatalf("count children after delete: %v", err)
	}
	// Retention has to be enforceable in one place, or it is not enforced.
	if after != 0 {
		t.Errorf("%d child rows survived the deletion of their run", after)
	}
}

func TestCleanupExpiredAppliesRetention(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, owner, err := s.NewAnonymousToken(ctx)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	runID, err := s.Save(ctx, owner, SavedRun{
		Intention:      "hope",
		ResearchStatus: "unavailable",
		PromptVersion:  "v1",
		Payload:        map[string]any{},
		Sources:        []SavedSource{{SourceID: "s1", Title: "T", URL: "https://example.org/t", Kind: "web_page"}},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := s.db.Exec(ctx,
		`UPDATE discovery_runs SET expires_at = now() - interval '1 day' WHERE id = $1`, runID); err != nil {
		t.Fatalf("expire: %v", err)
	}

	if _, err := s.CleanupExpired(ctx); err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if _, err := s.Get(ctx, owner, runID); !errors.Is(err, ErrForbidden) {
		t.Errorf("an expired run = %v, want ErrForbidden after cleanup", err)
	}

	// Running it twice is safe, which is what lets it be scheduled.
	if _, err := s.CleanupExpired(ctx); err != nil {
		t.Errorf("CleanupExpired is not idempotent: %v", err)
	}
}

// TestSaveSurvivesAnUnsupportedResearchStatus proves an unexpected value from
// a caller cannot fail a save against the column's CHECK constraint.
func TestSaveSurvivesAnUnsupportedResearchStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, owner, err := s.NewAnonymousToken(ctx)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	id, err := s.Save(ctx, owner, SavedRun{
		Intention:      "curiosity",
		ResearchStatus: "definitely-researched",
		PromptVersion:  "v1",
		Payload:        map[string]any{},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	run, err := s.Get(ctx, owner, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if run.ResearchStatus != "unavailable" {
		t.Errorf("researchStatus = %q, want the safe default", run.ResearchStatus)
	}
}

// TestSaveDropsAnUncitableSource: a source with no address cannot be followed,
// so it is not stored as one.
func TestSaveDropsAnUncitableSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, owner, err := s.NewAnonymousToken(ctx)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	id, err := s.Save(ctx, owner, SavedRun{
		Intention:      "curiosity",
		ResearchStatus: "researched",
		PromptVersion:  "v1",
		Payload:        map[string]any{},
		Sources: []SavedSource{
			{SourceID: "s1", Title: "Good", URL: "https://example.org/ok", Kind: "web_page"},
			{SourceID: "s2", Title: "No address", URL: "", Kind: "web_page"},
			{SourceID: "s3", Title: "", URL: "https://example.org/no-title", Kind: "web_page"},
		},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	run, err := s.Get(ctx, owner, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(run.Sources) != 1 || run.Sources[0].SourceID != "s1" {
		t.Errorf("sources = %+v, want only the citable one", run.Sources)
	}
}
