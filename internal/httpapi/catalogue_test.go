package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"everroot/internal/ai"
)

/* ------------------------------------------------------------------ stubs */

type stubTreeSource struct {
	facts []treeFact
	err   error
}

func (s stubTreeSource) List(context.Context, string, int, int) ([]treeFact, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.facts, nil
}

func (s stubTreeSource) Get(_ context.Context, id string) (treeFact, error) {
	if s.err != nil {
		return treeFact{}, s.err
	}
	for _, f := range s.facts {
		if f.SpeciesID == id {
			return f, nil
		}
	}
	return treeFact{}, errTreeNotFound
}

type stubExplainer struct {
	result ai.ExplainResult
	err    error
	calls  int
	last   ai.ExplainInput
}

func (s *stubExplainer) Explain(_ context.Context, in ai.ExplainInput) (ai.ExplainResult, error) {
	s.calls++
	s.last = in
	return s.result, s.err
}

const sampleSpeciesID = "b38a17b2-8b49-519a-9e83-4eb30cc97ce2"

func sampleFact() treeFact {
	return treeFact{
		SpeciesID:    sampleSpeciesID,
		AcceptedName: "Ficus religiosa",
		Authorship:   "L.",
		Family:       "Moraceae",
		Genus:        "Ficus",
		Order:        "Rosales",
		Rank:         "SPECIES",
		TaxonStatus:  "ACCEPTED",
		Protologue:   "L. (1753). In: Sp. Pl. 1059.",
		CommonNames:  []treeCommonName{{Name: "Peepul Tree", Language: "eng"}},
		Authority: treeAuthorityRef{
			Dataset: "GBIF Backbone Taxonomy", DOI: "10.15468/39omei",
			URL: "https://www.gbif.org/species/5361935", TaxonKey: 5361935,
		},
		Sources:       []sourceResult{{Title: "GBIF Backbone Taxonomy", VerificationStatus: "verified", Relationship: "identifies"}},
		RecordType:    "botanical_authority",
		CulturalClaim: "none",
	}
}

func newShimmedAPI(src treeSource, exp explainer) http.Handler {
	a := &API{
		mux:       http.NewServeMux(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		catalogue: src,
		explainer: exp,
	}
	a.routes()
	return a.mux
}

func postExplain(t *testing.T, h http.Handler, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/explain", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var decoded map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		decoded = map[string]any{}
	}
	return w, decoded
}

/* ------------------------------------------------------------ guardrails */

// collectKeys walks decoded JSON and reports every object key matching want.
func collectKeys(value any, want map[string]bool, found map[string]bool) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if want[key] {
				found[key] = true
			}
			collectKeys(child, want, found)
		}
	case []any:
		for _, child := range node {
			collectKeys(child, want, found)
		}
	}
}

// TestCataloguePayloadCarriesNoCulturalClaim is the guardrail the brief asks
// for: an unsupported cultural mapping must never appear as a verified record.
// Nothing in the exploratory payload may even have a field to put one in.
func TestCataloguePayloadCarriesNoCulturalClaim(t *testing.T) {
	h := newShimmedAPI(stubTreeSource{facts: []treeFact{sampleFact()}}, &stubExplainer{
		result: ai.ExplainResult{Text: "A botanical record only.", Model: "test-model"},
	})
	for _, tc := range []struct{ name, body string }{
		{"explore", `{"intention":"remember"}`},
		{"why_this_tree", fmt.Sprintf(`{"intention":"culture","speciesId":%q}`, sampleSpeciesID)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, decoded := postExplain(t, h, tc.body)
			if w.Code != 200 {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			found := map[string]bool{}
			collectKeys(decoded, map[string]bool{"claim": true, "interpretation": true, "originalClaim": true, "verifiedAssociation": true}, found)
			if len(found) > 0 {
				t.Fatalf("exploratory payload exposed cultural-claim fields %v: %s", found, w.Body.String())
			}
			for _, tree := range decoded["trees"].([]any) {
				entry := tree.(map[string]any)
				if entry["culturalClaim"] != "none" || entry["recordType"] != "botanical_authority" {
					t.Fatalf("catalogue entry is not labelled as a botanical record: %v", entry)
				}
			}
			evidence := decoded["evidence"].(map[string]any)
			if evidence["culturalAssociation"] == assocReviewedMatch {
				t.Fatalf("exploratory path claimed a reviewed cultural match without one: %v", evidence)
			}
		})
	}
}

/* --------------------------------------------------------- AI integration */

func TestExplainReportsValidGeneratedText(t *testing.T) {
	exp := &stubExplainer{result: ai.ExplainResult{Text: "  Grounded prose.  ", Model: "openai/gpt-oss-120b"}}
	h := newShimmedAPI(stubTreeSource{facts: []treeFact{sampleFact()}}, exp)
	_, decoded := postExplain(t, h, `{"intention":"personal"}`)

	envelope := decoded["explanation"].(map[string]any)
	if envelope["status"] != explainOK {
		t.Fatalf("expected ok, got %v", envelope)
	}
	if envelope["text"] != "Grounded prose." {
		t.Fatalf("text not trimmed: %q", envelope["text"])
	}
	if envelope["model"] != "openai/gpt-oss-120b" {
		t.Fatalf("model missing: %v", envelope)
	}
	if exp.calls != 1 {
		t.Fatalf("explainer called %d times", exp.calls)
	}
	// The provider receives only authority facts, never client-supplied prose.
	for _, s := range exp.last.Species {
		if s.AcceptedName != "Ficus religiosa" || s.Family != "Moraceae" {
			t.Fatalf("ungrounded facts sent: %+v", s)
		}
	}
}

func TestLiveExplainEndpointWithHuggingFace(t *testing.T) {
	if os.Getenv("HF_TOKEN") == "" {
		t.Skip("HF_TOKEN is not configured; skipping live Hugging Face endpoint test")
	}
	realClient, err := ai.New(nil)
	if err != nil {
		t.Fatalf("failed to create real ai client: %v", err)
	}
	h := newShimmedAPI(stubTreeSource{facts: []treeFact{sampleFact()}}, realClient)
	w, decoded := postExplain(t, h, `{"intention":"personal"}`)
	if w.Code != 200 {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	envelope := decoded["explanation"].(map[string]any)
	if envelope["status"] != explainOK {
		t.Fatalf("expected explainOK, got %v", envelope)
	}
	text, _ := envelope["text"].(string)
	if strings.TrimSpace(text) == "" {
		t.Fatalf("expected non-empty explanation text: %v", envelope)
	}
	model, _ := envelope["model"].(string)
	if !strings.Contains(model, "openai/gpt-oss-120b") {
		t.Fatalf("expected model containing openai/gpt-oss-120b, got %v", model)
	}
}

// A provider failure must never look like missing cultural data.
func TestExplainProviderFailureIsNotMissingData(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"error", ai.ErrProvider, reasonProvider},
		{"timeout", ai.ErrTimeout, reasonTimeout},
		{"unauthorized", ai.ErrUnauthorized, reasonUnauthorized},
		{"rate limited", ai.ErrRateLimited, reasonRateLimited},
		{"no completion", ai.ErrNoCompletion, reasonEmpty},
		{"bad response", ai.ErrBadResponse, reasonEmpty},
		{"wrapped timeout", fmt.Errorf("%w: %v", ai.ErrTimeout, context.DeadlineExceeded), reasonTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newShimmedAPI(stubTreeSource{facts: []treeFact{sampleFact()}}, &stubExplainer{err: tc.err})
			w, decoded := postExplain(t, h, `{"intention":"explore"}`)
			if w.Code != 200 {
				t.Fatalf("provider failure must not fail the request: %d", w.Code)
			}
			envelope := decoded["explanation"].(map[string]any)
			if envelope["status"] != explainUnavailable || envelope["reason"] != tc.want {
				t.Fatalf("expected unavailable/%s, got %v", tc.want, envelope)
			}
			// The sourced botanical records still come back, so the page has
			// something true to show.
			if len(decoded["trees"].([]any)) == 0 {
				t.Fatal("provider failure discarded the botanical records")
			}
			evidence := decoded["evidence"].(map[string]any)
			if evidence["culturalAssociation"] == assocReviewedMatch {
				t.Fatalf("provider failure leaked a cultural claim: %v", evidence)
			}
		})
	}
}

// A model that replies with empty text is a provider problem, not an empty
// dataset, and must say so.
func TestExplainEmptyModelReplyIsFlagged(t *testing.T) {
	h := newShimmedAPI(stubTreeSource{facts: []treeFact{sampleFact()}}, &stubExplainer{
		result: ai.ExplainResult{Text: "   ", Model: "test-model"},
	})
	_, decoded := postExplain(t, h, `{"intention":"explore"}`)
	envelope := decoded["explanation"].(map[string]any)
	if envelope["status"] != explainUnavailable || envelope["reason"] != reasonEmpty {
		t.Fatalf("empty model reply not flagged: %v", envelope)
	}
}

func TestExplainWithoutProviderSaysSo(t *testing.T) {
	h := newShimmedAPI(stubTreeSource{facts: []treeFact{sampleFact()}}, nil)
	_, decoded := postExplain(t, h, `{"intention":"explore"}`)
	envelope := decoded["explanation"].(map[string]any)
	if envelope["status"] != explainUnavailable || envelope["reason"] != reasonNotConfigured {
		t.Fatalf("unconfigured provider not reported: %v", envelope)
	}
}

/* ------------------------------------------------------------- validation */

func TestExplainRejectsBadInput(t *testing.T) {
	h := newShimmedAPI(stubTreeSource{facts: []treeFact{sampleFact()}}, &stubExplainer{})
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"unknown intention", `{"intention":"hack"}`, 400},
		{"missing intention", `{"speciesId":""}`, 400},
		{"client cannot inject facts", `{"intention":"explore","facts":{"acceptedName":"Invented"}}`, 400},
		{"unknown species", `{"intention":"explore","speciesId":"00000000-0000-0000-0000-000000000000"}`, 404},
		{"malformed species id", `{"intention":"explore","speciesId":"not-a-uuid"}`, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := postExplain(t, h, tc.body)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

/* -------------------------------------------------------------- catalogue */

func TestTreesEndpointShape(t *testing.T) {
	h := newShimmedAPI(stubTreeSource{facts: []treeFact{sampleFact()}}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trees?q=ficus", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	var body struct {
		Status   string     `json:"status"`
		Ordering string     `json:"ordering"`
		Query    string     `json:"query"`
		Count    int        `json:"count"`
		Trees    []treeFact `json:"trees"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Count != 1 || len(body.Trees) != 1 {
		t.Fatalf("unexpected payload: %s", w.Body.String())
	}
	if body.Ordering == "" {
		t.Fatal("ordering must be stated so nothing looks like a ranking")
	}
	if body.Trees[0].Authority.TaxonKey == 0 {
		t.Fatal("authority taxon key missing")
	}
}

// An empty catalogue is an empty result, not an error.
func TestTreesEndpointEmptyCatalogue(t *testing.T) {
	h := newShimmedAPI(stubTreeSource{facts: []treeFact{}}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	var body struct {
		Count int        `json:"count"`
		Trees []treeFact `json:"trees"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 0 || len(body.Trees) != 0 {
		t.Fatalf("expected empty catalogue, got %s", w.Body.String())
	}
}

func TestTreesEndpointRejectsBadPaging(t *testing.T) {
	h := newShimmedAPI(stubTreeSource{facts: []treeFact{}}, nil)
	for _, query := range []string{"limit=0", "limit=101", "limit=abc", "offset=-1", "q=" + strings.Repeat("x", 200)} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/trees?"+query, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("%s returned %d", query, w.Code)
		}
	}
}

// A catalogue outage must not be reported as "no trees exist".
func TestTreesEndpointReportsOutage(t *testing.T) {
	h := newShimmedAPI(stubTreeSource{err: errors.New("connection reset")}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trees", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("status=%d want 503", w.Code)
	}
}

// TestGeneralExplanationSurvivesACatalogueOutage replaces an assertion that a
// readable catalogue was a precondition for answering. It was not: a general
// explanation is about the visitor's own intention and cultural context, and
// turning a database outage into a 503 made a broken connection look like the
// visitor had asked something unanswerable. The outage is still reported —
// plainly, in the evidence panel — it just no longer silences the model.
func TestGeneralExplanationSurvivesACatalogueOutage(t *testing.T) {
	h := newShimmedAPI(stubTreeSource{err: errors.New("connection reset")}, &stubExplainer{})
	w, body := postExplain(t, h, `{"intention":"explore"}`)
	if w.Code != 200 {
		t.Fatalf("status=%d want 200: an unreadable catalogue must not block a general explanation", w.Code)
	}
	evidence, _ := body["evidence"].(map[string]any)
	if evidence == nil {
		t.Fatalf("no evidence object in %s", w.Body.String())
	}
	if got := evidence["botanicalIdentity"]; got != "unavailable" {
		t.Errorf("botanicalIdentity=%v, want \"unavailable\": the outage must be stated, not hidden", got)
	}
	if got := evidence["habitat"]; got != "no_reviewed_record" {
		t.Errorf("habitat=%v, want \"no_reviewed_record\"", got)
	}
}

// TestGeneralExplanationNeedsNoCatalogueAtAll covers the other half: a
// deployment with no catalogue configured at all still explains.
func TestGeneralExplanationNeedsNoCatalogueAtAll(t *testing.T) {
	h := newShimmedAPI(nil, &stubExplainer{})
	w, _ := postExplain(t, h, `{"intention":"explore","tradition":"Sikh","nakshatra":"Ashwini"}`)
	if w.Code != 200 {
		t.Fatalf("status=%d want 200", w.Code)
	}
}

// TestExplainForARegisteredSpeciesStillValidates keeps the distinction the
// brief asks for. Naming a species is a factual claim about a record, so it is
// still checked: an unknown one is a 404, and an unreadable catalogue is still
// a 503, because "I could not look it up" and "there is no such record" are
// different answers and only one of them may be given by guessing.
func TestExplainForARegisteredSpeciesStillValidates(t *testing.T) {
	t.Run("unknown species is a 404", func(t *testing.T) {
		h := newShimmedAPI(stubTreeSource{facts: []treeFact{{SpeciesID: "azadirachta-indica"}}}, &stubExplainer{})
		w, _ := postExplain(t, h, `{"intention":"explore","speciesId":"quercus-robur"}`)
		if w.Code != 404 {
			t.Fatalf("status=%d want 404", w.Code)
		}
	})

	t.Run("unreadable catalogue is a 503", func(t *testing.T) {
		h := newShimmedAPI(stubTreeSource{err: errors.New("connection reset")}, &stubExplainer{})
		w, _ := postExplain(t, h, `{"intention":"explore","speciesId":"azadirachta-indica"}`)
		if w.Code != 503 {
			t.Fatalf("status=%d want 503: a named species cannot be validated against an unreadable record", w.Code)
		}
	})

	t.Run("a known species is answered", func(t *testing.T) {
		h := newShimmedAPI(stubTreeSource{facts: []treeFact{{SpeciesID: "azadirachta-indica", AcceptedName: "Azadirachta indica"}}}, &stubExplainer{})
		w, _ := postExplain(t, h, `{"intention":"explore","speciesId":"azadirachta-indica"}`)
		if w.Code != 200 {
			t.Fatalf("status=%d want 200", w.Code)
		}
	})
}

// TestExplainNeverClaimsAReviewItDoesNotHold is the guardrail that survives the
// change: an unregistered tradition may now be explained, but nothing in the
// reply may present that as a reviewed cultural association.
func TestExplainNeverClaimsAReviewItDoesNotHold(t *testing.T) {
	h := newShimmedAPI(nil, &stubExplainer{})
	_, body := postExplain(t, h, `{"intention":"explore","tradition":"Sikh","nakshatra":"Ashwini"}`)
	evidence, _ := body["evidence"].(map[string]any)
	if evidence == nil {
		t.Fatalf("no evidence object in %s", w(body))
	}
	if got := evidence["culturalAssociation"]; got == "reviewed_match" {
		t.Error("an unregistered tradition was reported as a reviewed match")
	}
	if got := evidence["culturalAssociation"]; got != "no_reviewed_match" {
		t.Errorf("culturalAssociation=%v, want \"no_reviewed_match\"", got)
	}
}

func w(body map[string]any) string {
	raw, _ := json.Marshal(body)
	return string(raw)
}

/* ---------------------------------------------------- prompt guardrails */

func TestEscapeLikeNeutralisesWildcards(t *testing.T) {
	if got := likeEscaper.Replace("100%_fig\\"); got != `100\%\_fig\\` {
		t.Fatalf("wildcards not escaped: %q", got)
	}
}
