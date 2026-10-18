package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoopbackAddress(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{{"127.0.0.1:1234", true}, {"[::1]:1234", true}, {"192.0.2.2:1234", false}, {"invalid", false}} {
		if got := isLoopbackAddr(tc.addr); got != tc.want {
			t.Errorf("isLoopbackAddr(%q)=%v want %v", tc.addr, got, tc.want)
		}
	}
}

func TestAdminRoutesFailClosed(t *testing.T) {
	t.Setenv("ADMIN_LOCAL_ENABLED", "false")
	t.Setenv("ADMIN_REVIEWER_ID", "")
	t.Setenv("ADMIN_REVIEWER_ROLE", "")
	t.Setenv("HF_TOKEN", "test-token")
	h, err := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/internal/v1/review/queue", nil)
	r.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("disabled admin route returned %d", w.Code)
	}
}

func TestPublicDiscoveryRequiresKnownValidInput(t *testing.T) {
	t.Setenv("HF_TOKEN", "test-token")
	h, err := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/discoveries", strings.NewReader(`{"tradition":"","nakshatra":"Ashwini"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("invalid anonymous discovery returned %d", w.Code)
	}
}

// This test runs only against an explicitly supplied, migrated, disposable test database.
// Set TEST_DATABASE_URL before `go test ./...` to exercise the full workflow.
func TestPostgresDiscoveryAndReviewWorkflow(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	// The explanation layer must be optional. With no provider configured the
	// API still serves reviewed records and botanical facts, and reports the
	// missing provider as its own state. This also keeps the test offline.
	t.Setenv("HF_TOKEN", "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	api, err := New(pool, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADMIN_LOCAL_ENABLED", "true")
	t.Setenv("ADMIN_REVIEWER_ID", "test-reviewer")
	t.Setenv("ADMIN_REVIEWER_ROLE", "cultural")
	post := func(path string, body any, role string, enable bool, remote string) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
		req.RemoteAddr = remote
		if role != "" {
			t.Setenv("ADMIN_REVIEWER_ROLE", role)
		}
		if enable {
			t.Setenv("ADMIN_LOCAL_ENABLED", "true")
		} else {
			t.Setenv("ADMIN_LOCAL_ENABLED", "false")
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, req)
		return w
	}
	get := func(path string, remote string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = remote
		w := httptest.NewRecorder()
		api.ServeHTTP(w, req)
		return w
	}
	regionsResponse := get("/api/v1/traditions/nakshatra-tree-association/regions", "127.0.0.1:4123")
	if regionsResponse.Code != 200 || !strings.Contains(regionsResponse.Body.String(), "Tamil Nadu") {
		t.Fatalf("tradition regions response: %d %s", regionsResponse.Code, regionsResponse.Body.String())
	}
	// A remote caller and a botanical reviewer cannot use cultural review endpoints.
	if got := get("/internal/v1/review/queue", "192.0.2.10:4512"); got.Code != 403 {
		t.Fatalf("remote review access status=%d", got.Code)
	}
	if got := post("/internal/v1/review/00000000-0000-0000-0000-000000000001", map[string]string{"decision": "approved"}, "botanical", true, "127.0.0.1:4123"); got.Code != 403 {
		t.Fatalf("botanical reviewer cultural review status=%d", got.Code)
	}
	t.Setenv("ADMIN_REVIEWER_ROLE", "cultural")

	created := post("/internal/v1/sources", map[string]any{"title": "Synthetic test source", "sourceType": "test_fixture", "author": "Test", "publisher": "Everroot test suite", "publicationDate": "2020-01-01", "bibliographicReference": "Synthetic fixture; not a cultural source", "language": "en", "accessedOn": "2026-10-15", "attributionRequirements": "Test-only", "reuseStatus": "citation_only"}, "cultural", true, "127.0.0.1:4123")
	if created.Code != 201 {
		t.Fatalf("create source: %d %s", created.Code, created.Body.String())
	}
	var src map[string]string
	_ = json.Unmarshal(created.Body.Bytes(), &src)
	if _, err = pool.Exec(ctx, `UPDATE sources SET verification_status='verified' WHERE id=$1`, src["id"]); err == nil {
		t.Fatal("source verification changed without review history")
	}
	protectedText := "Test excerpt that cannot be stored under citation-only rights"
	if refused := post("/internal/v1/sources/"+src["id"]+"/evidence", map[string]any{"locator": "restricted test locator", "language": "en", "originalText": protectedText, "translationRights": "not_applicable"}, "cultural", true, "127.0.0.1:4123"); refused.Code != 400 {
		t.Fatalf("citation-only passage storage status=%d", refused.Code)
	}
	ev := post("/internal/v1/sources/"+src["id"]+"/evidence", map[string]any{"locator": "test-only locator", "language": "en", "originalText": nil, "translationRights": "not_applicable", "evidenceNotes": "Synthetic test evidence; not real cultural content"}, "cultural", true, "127.0.0.1:4123")
	if ev.Code != 201 {
		t.Fatalf("create evidence: %d %s", ev.Code, ev.Body.String())
	}
	var evidence map[string]string
	_ = json.Unmarshal(ev.Body.Bytes(), &evidence)
	makeMap := func(claim string) map[string]any {
		return map[string]any{"tradition": "nakshatra-tree-association", "regionCode": "IN-TN", "nakshatra": "Ashwini", "claim": claim, "evidence": []any{map[string]string{"id": evidence["id"], "relationship": "supports"}}, "botanicalCandidates": []any{map[string]any{"scientificName": "Testus alpha", "certainty": "confirmed", "confidence": 0.9, "evidence": []any{map[string]string{"id": evidence["id"], "relationship": "supports"}}}, map[string]any{"scientificName": "Testus beta", "certainty": "confirmed", "confidence": 0.7, "evidence": []any{map[string]string{"id": evidence["id"], "relationship": "qualifies"}}}}}
	}
	var revisions []map[string]string
	var botanicalIDs []string
	for i, claim := range []string{"Synthetic variant A (test only)", "Synthetic variant B (test only)"} {
		createdMap := post("/internal/v1/mappings", makeMap(claim), "cultural", true, "127.0.0.1:4123")
		if createdMap.Code != 201 {
			t.Fatalf("create mapping: %d %s", createdMap.Code, createdMap.Body.String())
		}
		var data map[string]any
		_ = json.Unmarshal(createdMap.Body.Bytes(), &data)
		revisions = append(revisions, map[string]string{"mappingId": data["mappingId"].(string), "revisionId": data["revisionId"].(string)})
		if i == 0 {
			for _, candidateID := range data["botanicalCandidateIds"].([]any) {
				botanicalIDs = append(botanicalIDs, candidateID.(string))
			}
		}
		// Database and API publication gates reject a reviewed mapping with unverified sources.
		approved := post("/internal/v1/review/"+revisions[len(revisions)-1]["revisionId"], map[string]string{"decision": "approved", "notes": "test cultural review"}, "cultural", true, "127.0.0.1:4123")
		if approved.Code != 204 {
			t.Fatalf("review mapping: %d %s", approved.Code, approved.Body.String())
		}
		if i == 0 {
			gate := post("/internal/v1/publications/"+revisions[len(revisions)-1]["revisionId"], map[string]string{"action": "published", "reason": "test"}, "cultural", true, "127.0.0.1:4123")
			if gate.Code != 400 {
				t.Fatalf("publication gate status=%d, expected 400", gate.Code)
			}
			verified := post("/internal/v1/sources/"+src["id"]+"/review", map[string]string{"decision": "approved", "notes": "synthetic test verification"}, "cultural", true, "127.0.0.1:4123")
			if verified.Code != 204 {
				t.Fatalf("verify source: %d %s", verified.Code, verified.Body.String())
			}
		}
		pub := post("/internal/v1/publications/"+revisions[len(revisions)-1]["revisionId"], map[string]string{"action": "published", "reason": "test fixture only"}, "cultural", true, "127.0.0.1:4123")
		if pub.Code != 204 {
			t.Fatalf("publish mapping: %d %s", pub.Code, pub.Body.String())
		}
	}
	// A material edit creates version 2. Version 1 remains public until version 2 is explicitly approved and published.
	version2 := makeMap("Synthetic revision A v2 (test only)")
	version2["mappingId"] = revisions[0]["mappingId"]
	createdV2 := post("/internal/v1/mappings", version2, "cultural", true, "127.0.0.1:4123")
	if createdV2.Code != 201 {
		t.Fatalf("create revision 2: %d %s", createdV2.Code, createdV2.Body.String())
	}
	var v2 map[string]any
	_ = json.Unmarshal(createdV2.Body.Bytes(), &v2)
	if v2["version"].(float64) != 2 {
		t.Fatalf("expected revision 2, got %v", v2["version"])
	}
	botanicalIDs = botanicalIDs[:0]
	for _, candidateID := range v2["botanicalCandidateIds"].([]any) {
		botanicalIDs = append(botanicalIDs, candidateID.(string))
	}
	approvedV2 := post("/internal/v1/review/"+v2["revisionId"].(string), map[string]string{"decision": "approved", "notes": "review v2"}, "cultural", true, "127.0.0.1:4123")
	if approvedV2.Code != 204 {
		t.Fatalf("review v2: %d %s", approvedV2.Code, approvedV2.Body.String())
	}
	publishedV2 := post("/internal/v1/publications/"+v2["revisionId"].(string), map[string]string{"action": "published", "reason": "explicit version switch"}, "cultural", true, "127.0.0.1:4123")
	if publishedV2.Code != 204 {
		t.Fatalf("publish v2: %d %s", publishedV2.Code, publishedV2.Body.String())
	}
	if denied := post("/internal/v1/review/"+v2["revisionId"].(string), map[string]string{"decision": "changes_requested", "notes": "must use a new revision"}, "cultural", true, "127.0.0.1:4123"); denied.Code != 400 {
		t.Fatalf("published revision re-review status=%d", denied.Code)
	}
	if denied := post("/internal/v1/publications/"+v2["revisionId"].(string), map[string]string{"action": "unpublished", "reason": "role check"}, "botanical", true, "127.0.0.1:4123"); denied.Code != 403 {
		t.Fatalf("botanical reviewer publication status=%d", denied.Code)
	}
	version3 := makeMap("Synthetic revision A v3 (test only)")
	version3["mappingId"] = revisions[0]["mappingId"]
	createdV3 := post("/internal/v1/mappings", version3, "cultural", true, "127.0.0.1:4123")
	if createdV3.Code != 201 {
		t.Fatalf("create revision 3: %d %s", createdV3.Code, createdV3.Body.String())
	}
	var v3 map[string]any
	_ = json.Unmarshal(createdV3.Body.Bytes(), &v3)
	if v3["version"].(float64) != 3 {
		t.Fatalf("expected revision 3, got %v", v3["version"])
	}
	if blocked := post("/internal/v1/publications/"+v2["revisionId"].(string), map[string]string{"action": "published", "reason": "old revision must remain immutable"}, "cultural", true, "127.0.0.1:4123"); blocked.Code != 400 {
		t.Fatalf("old revision republish status=%d", blocked.Code)
	}
	if reviewed := post("/internal/v1/review/"+v3["revisionId"].(string), map[string]string{"decision": "approved", "notes": "first pass"}, "cultural", true, "127.0.0.1:4123"); reviewed.Code != 204 {
		t.Fatalf("review v3: %d %s", reviewed.Code, reviewed.Body.String())
	}
	if reviewed := post("/internal/v1/review/"+v3["revisionId"].(string), map[string]string{"decision": "changes_requested", "notes": "approval must be superseded"}, "cultural", true, "127.0.0.1:4123"); reviewed.Code != 204 {
		t.Fatalf("request changes v3: %d %s", reviewed.Code, reviewed.Body.String())
	}
	if blocked := post("/internal/v1/publications/"+v3["revisionId"].(string), map[string]string{"action": "published", "reason": "must fail after latest decision"}, "cultural", true, "127.0.0.1:4123"); blocked.Code != 400 {
		t.Fatalf("latest review decision gate status=%d", blocked.Code)
	}
	for _, candidateID := range botanicalIDs {
		reviewed := post("/internal/v1/botanical-candidates/"+candidateID+"/review", map[string]string{"decision": "approved", "notes": "separate botanical test review"}, "botanical", true, "127.0.0.1:4123")
		if reviewed.Code != 204 {
			t.Fatalf("botanical review: %d %s", reviewed.Code, reviewed.Body.String())
		}
	}
	if len(botanicalIDs) > 0 {
		if _, err = pool.Exec(ctx, `UPDATE botanical_candidates SET certainty='uncertain' WHERE id=$1`, botanicalIDs[0]); err == nil {
			t.Fatal("botanical candidate revision mutated in place")
		}
	}
	missingEvidenceTx, txErr := pool.Begin(ctx)
	if txErr != nil {
		t.Fatal(txErr)
	}
	var orphanSpecies string
	if txErr = missingEvidenceTx.QueryRow(ctx, `INSERT INTO tree_species(scientific_name) VALUES('Testus orphan') RETURNING id`).Scan(&orphanSpecies); txErr == nil {
		_, txErr = missingEvidenceTx.Exec(ctx, `INSERT INTO botanical_candidates(revision_id,species_id,certainty,confidence) VALUES($1,$2,'probable',0.4)`, v2["revisionId"].(string), orphanSpecies)
	}
	if txErr == nil {
		txErr = missingEvidenceTx.Commit(ctx)
	}
	if txErr != nil {
		_ = missingEvidenceTx.Rollback(ctx)
	} else {
		_ = missingEvidenceTx.Rollback(ctx)
		t.Fatal("botanical candidate without independent evidence committed")
	}
	if _, err = pool.Exec(ctx, `UPDATE source_evidence SET locator='changed' WHERE id=$1`, evidence["id"]); err == nil {
		t.Fatal("source evidence mutated in place")
	}
	if _, err = pool.Exec(ctx, `UPDATE sources SET title='changed' WHERE id=$1`, src["id"]); err == nil {
		t.Fatal("source bibliography mutated in place")
	}
	if _, err = pool.Exec(ctx, `UPDATE tree_species SET family='changed' WHERE scientific_name='Testus alpha'`); err == nil {
		t.Fatal("referenced botanical identity mutated in place")
	}
	t.Setenv("ADMIN_REVIEWER_ROLE", "cultural")
	// Anonymous discovery returns both competing reviewed records, without a session or account.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/discoveries", strings.NewReader(`{"tradition":"nakshatra-tree-association","region":"Tamil Nadu","nakshatra":"Ashwini"}`))
	request.RemoteAddr = "127.0.0.1:7788"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("anonymous discovery: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Status          string `json:"status"`
		Recommendations []struct {
			Claim     string `json:"claim"`
			Botanical []struct {
				Definitive bool `json:"definitive"`
			} `json:"botanicalCandidates"`
		} `json:"recommendations"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "verified_results" || len(result.Recommendations) < 2 {
		t.Fatalf("conflicting results were collapsed or omitted: %+v", result)
	}
	claims := map[string]bool{}
	for _, item := range result.Recommendations {
		claims[item.Claim] = true
	}
	if !claims["Synthetic revision A v2 (test only)"] || claims["Synthetic revision A v3 (test only)"] {
		t.Fatalf("published revision changed without publication: claims=%v", claims)
	}
	for _, x := range result.Recommendations {
		for _, b := range x.Botanical {
			if b.Definitive {
				t.Fatal("multiple candidate identifications must not be definitive")
			}
		}
	}

	// ---- Track B: exploratory botanical discovery -------------------------
	// The catalogue must expose only authority-verified botanical records, and
	// must never carry a field in which a cultural claim could be smuggled in.
	catalogueResponse := get("/api/v1/trees", "127.0.0.1:7788")
	if catalogueResponse.Code != 200 {
		t.Fatalf("catalogue: %d %s", catalogueResponse.Code, catalogueResponse.Body.String())
	}
	var catalogue struct {
		Status   string `json:"status"`
		Ordering string `json:"ordering"`
		Count    int    `json:"count"`
		Trees    []struct {
			AcceptedName  string `json:"acceptedName"`
			TaxonStatus   string `json:"taxonStatus"`
			RecordType    string `json:"recordType"`
			CulturalClaim string `json:"culturalClaim"`
			Authority     struct {
				Dataset  string `json:"dataset"`
				TaxonKey int64  `json:"taxonKey"`
			} `json:"authority"`
		} `json:"trees"`
	}
	if err = json.Unmarshal(catalogueResponse.Body.Bytes(), &catalogue); err != nil {
		t.Fatal(err)
	}
	if catalogue.Status != "ok" || catalogue.Count == 0 || catalogue.Ordering == "" {
		t.Fatalf("catalogue payload unusable: %s", catalogueResponse.Body.String())
	}
	for _, tree := range catalogue.Trees {
		if tree.RecordType != "botanical_authority" || tree.CulturalClaim != "none" {
			t.Fatalf("catalogue entry is not labelled as a botanical record: %+v", tree)
		}
		if tree.Authority.TaxonKey == 0 || tree.Authority.Dataset == "" {
			t.Fatalf("catalogue entry lost its authority reference: %+v", tree)
		}
		if tree.TaxonStatus != "ACCEPTED" {
			t.Fatalf("catalogue entry is not an accepted name: %+v", tree)
		}
	}
	var decodedCatalogue any
	_ = json.Unmarshal(catalogueResponse.Body.Bytes(), &decodedCatalogue)
	claimFields := map[string]bool{}
	collectKeys(decodedCatalogue, map[string]bool{"claim": true, "interpretation": true, "originalClaim": true}, claimFields)
	if len(claimFields) > 0 {
		t.Fatalf("catalogue exposed cultural-claim fields %v", claimFields)
	}

	// Search reaches common names, and a search with no match is an empty
	// result rather than an error.
	if got := get("/api/v1/trees?q=khejri", "127.0.0.1:7788"); got.Code != 200 || !strings.Contains(got.Body.String(), "Prosopis cineraria") {
		t.Fatalf("common-name search failed: %d %s", got.Code, got.Body.String())
	}
	if got := get("/api/v1/trees?q=zzzznotarealplant", "127.0.0.1:7788"); got.Code != 200 || !strings.Contains(got.Body.String(), `"count":0`) {
		t.Fatalf("empty catalogue search: %d %s", got.Code, got.Body.String())
	}

	// The explanation endpoint is the only place a cultural match may be
	// claimed, and it must use the same gate the cultural endpoint uses.
	explain := func(body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/explain", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:7788"
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		api.ServeHTTP(resp, req)
		var decoded map[string]any
		_ = json.Unmarshal(resp.Body.Bytes(), &decoded)
		return resp.Code, decoded
	}
	associationOf := func(decoded map[string]any) string {
		evidence, _ := decoded["evidence"].(map[string]any)
		value, _ := evidence["culturalAssociation"].(string)
		return value
	}
	providerReasonOf := func(decoded map[string]any) string {
		envelope, _ := decoded["explanation"].(map[string]any)
		value, _ := envelope["reason"].(string)
		return value
	}

	// 1. A matching reviewed record exists for this region.
	code, matched := explain(`{"intention":"culture","tradition":"nakshatra-tree-association","nakshatra":"Ashwini","region":"Tamil Nadu"}`)
	if code != 200 {
		t.Fatalf("explain (matching region) status=%d", code)
	}
	if associationOf(matched) != assocReviewedMatch {
		t.Fatalf("a published reviewed record was not reported as a match: %v", matched["evidence"])
	}

	// 2. A reviewed record exists, but for a different region.
	code, otherRegion := explain(`{"intention":"culture","tradition":"nakshatra-tree-association","nakshatra":"Ashwini","region":"North India"}`)
	if code != 200 || associationOf(otherRegion) != assocNoReviewedMatch {
		t.Fatalf("region mismatch not reported honestly: %d %v", code, otherRegion["evidence"])
	}

	// 3. No reviewed record for the selection, and nothing invented to fill it.
	code, unmatched := explain(`{"intention":"explore","tradition":"nakshatra-tree-association","nakshatra":"Revati","region":""}`)
	if code != 200 || associationOf(unmatched) != assocNoReviewedMatch {
		t.Fatalf("absent cultural record not reported honestly: %d %v", code, unmatched["evidence"])
	}

	// 4. A selection that cannot be queried at all is not the same as a miss.
	code, notQueryable := explain(`{"intention":"remember"}`)
	if code != 200 || associationOf(notQueryable) != assocNotQueryable {
		t.Fatalf("unqueryable selection misreported: %d %v", code, notQueryable["evidence"])
	}

	// 5. The provider is unconfigured here, which is a provider state and never
	//    a statement about the cultural dataset.
	if associationOf(notQueryable) == assocReviewedMatch || providerReasonOf(notQueryable) != reasonNotConfigured {
		t.Fatalf("provider state conflated with data state: %v", notQueryable)
	}

	// 6. Unsupported input is rejected rather than answered with a guess.
	if code, _ := explain(`{"intention":"explore","tradition":"no-such-tradition","nakshatra":"Ashwini"}`); code != 400 {
		t.Fatalf("unsupported tradition accepted: %d", code)
	}

	// 7. The trees returned alongside an explanation are botanical records, and
	//    every one of them is still available even with no provider configured.
	for _, tree := range notQueryable["trees"].([]any) {
		entry := tree.(map[string]any)
		if entry["culturalClaim"] != "none" {
			t.Fatalf("exploratory tree carried a cultural claim: %v", entry)
		}
	}

	// Revision rows cannot be silently edited in place.
	if _, err = pool.Exec(ctx, `UPDATE mapping_revisions SET original_claim='mutation' WHERE id=$1`, revisions[0]["revisionId"]); err == nil {
		t.Fatal("mapping revision mutation unexpectedly succeeded")
	}
	var auditCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE actor_id='test-reviewer' AND action IN ('review','published','source_review','botanical_review')`).Scan(&auditCount); err != nil || auditCount == 0 {
		t.Fatalf("review/publication audit events missing (count=%d, err=%v)", auditCount, err)
	}
	revoked := post("/internal/v1/sources/"+src["id"]+"/review", map[string]string{"decision": "rejected", "notes": "test invalidation"}, "cultural", true, "127.0.0.1:4123")
	if revoked.Code != 204 {
		t.Fatalf("source revocation: %d %s", revoked.Code, revoked.Body.String())
	}
	var withdrawn int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM publication_events WHERE mapping_id=ANY($1::uuid[]) AND action='withdrawn'`, []string{revisions[0]["mappingId"], revisions[1]["mappingId"]}).Scan(&withdrawn); err != nil || withdrawn < 2 {
		t.Fatalf("source invalidation did not withdraw affected public mappings (count=%d, err=%v)", withdrawn, err)
	}
}
