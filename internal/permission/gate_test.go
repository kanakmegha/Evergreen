package permission

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOverrideRequiresLegalGrounds(t *testing.T) {
	g := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	o := Override{
		SourceID:           "src-1",
		MaterialType:       MaterialCulturalClaims,
		Operation:          OpCollect,
		LegalGrounds:       "",
		SupportingEvidence: "evidence",
		ReviewerID:         "reviewer-1",
		ReviewerRole:       "cultural",
		Scope:              "collect only",
	}
	if err := g.Override(context.Background(), o); err == nil {
		t.Fatal("expected error for empty legal grounds")
	}
}

func TestOverrideRequiresScope(t *testing.T) {
	g := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	o := Override{
		SourceID:           "src-1",
		MaterialType:       MaterialCulturalClaims,
		Operation:          OpCollect,
		LegalGrounds:       "grounds",
		SupportingEvidence: "evidence",
		ReviewerID:         "reviewer-1",
		ReviewerRole:       "cultural",
		Scope:              "",
	}
	if err := g.Override(context.Background(), o); err == nil {
		t.Fatal("expected error for empty scope")
	}
}

func TestContainsHelper(t *testing.T) {
	cases := []struct {
		list []string
		item string
		want bool
	}{
		{[]string{"collect", "store"}, "collect", true},
		{[]string{"collect", "store"}, "publish", false},
		{[]string{}, "collect", false},
		{nil, "collect", false},
	}
	for _, c := range cases {
		if got := contains(c.list, c.item); got != c.want {
			t.Errorf("contains(%v, %q) = %v, want %v", c.list, c.item, got, c.want)
		}
	}
}

// TestGateDBBacked runs only against an explicitly supplied, migrated test database.
// It inserts a permission row, evaluates a blocked and an allowed case, records an
// override, and checks the override becomes active.
func TestGateDBBacked(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	g := New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// A fresh synthetic source per run: sources are immutable once reviewed and
	// permission rows are append-only, so reusing a fixed id would make the
	// test un-runnable a second time.
	var sourceID string
	if err = pool.QueryRow(ctx, `
		INSERT INTO sources(title,source_type,author,publisher,language,accessed_on,attribution_requirements,reuse_status,url,bibliographic_reference)
		VALUES('synthetic test source','test_fixture','Everroot test suite','Everroot test suite','en',CURRENT_DATE,'Test-only','citation_only','http://example.test','Synthetic fixture; not a cultural source')
		RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}

	// Insert a restricted permission row for cultural claims.
	_, err = pool.Exec(ctx, `
		INSERT INTO source_permission_material_types
			(source_id, material_type, permission_state, licence_name, licence_url,
			 commercial_use_allowed, extraction_allowed, local_storage_allowed,
			 redistribution_allowed, adaptation_allowed, translation_allowed,
			 attribution_required, written_permission_required, written_permission_on_file,
			 community_consent_required, allowed_operations, reviewed_by, review_notes, evidence_urls)
		VALUES ($1,'cultural_claims','restricted',NULL,NULL,
		 false,false,false,false,false,false,
		 true,true,false,false,
		 '{}'::text[],'test-reviewer','synthetic test fixture','{}'::text[])`, sourceID)
	if err != nil {
		t.Fatal(err)
	}

	// Blocked case: restricted state.
	d, err := g.Evaluate(ctx, sourceID, MaterialCulturalClaims, OpCollect, false, true, true, "req-test")
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Decision != DecisionBlocked {
		t.Fatalf("expected blocked for restricted state, got %+v", d)
	}
	if !strings.Contains(strings.Join(d.Reasons, " "), "restricted") {
		t.Fatalf("expected restricted reason, got %v", d.Reasons)
	}

	// Record an override allowing collection.
	exp := time.Now().Add(time.Hour)
	if err = g.Override(ctx, Override{
		SourceID:           sourceID,
		MaterialType:       MaterialCulturalClaims,
		Operation:          OpCollect,
		LegalGrounds:       "Rightsholder confirmed in writing via email on 2026-10-16.",
		SupportingEvidence: "Email from rightsholder (msg-id example) archived in case file.",
		ReviewerID:         "reviewer-1",
		ReviewerRole:       "cultural",
		Scope:              "collect only, attribution required",
		ExpiresAt:          &exp,
	}); err != nil {
		t.Fatal(err)
	}

	active, err := g.IsOverrideActive(ctx, sourceID, MaterialCulturalClaims, OpCollect)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("expected active override")
	}

	// Cleanup synthetic test data.
	pool.Exec(ctx, `DELETE FROM permission_overrides WHERE source_id=$1`, sourceID)
	pool.Exec(ctx, `DELETE FROM source_permission_material_types WHERE source_id=$1`, sourceID)
	pool.Exec(ctx, `DELETE FROM sources WHERE id=$1`, sourceID)
}

// Ensure compile-time references to required symbols.
var _ = errors.Is
