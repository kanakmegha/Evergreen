package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"everroot/internal/ai"
	"everroot/internal/discovery"
	"everroot/internal/research"
)

// stubDiscoverer stands in for the configured model, so these tests spend no
// quota and never reach a network.
type stubDiscoverer struct {
	result ai.DiscoverResult
	err    error
	got    ai.DiscoverInput
}

func (s *stubDiscoverer) Discover(_ context.Context, in ai.DiscoverInput) (ai.DiscoverResult, error) {
	s.got = in
	if s.err != nil {
		return ai.DiscoverResult{}, s.err
	}
	return s.result, nil
}

// stubResearcher is a research.Provider double.
type stubResearcher struct {
	sources []research.Source
	err     error
	got     research.Query
}

func (s *stubResearcher) Name() string { return "stub" }
func (s *stubResearcher) Search(_ context.Context, q research.Query, _ int) ([]research.Source, error) {
	s.got = q
	return s.sources, s.err
}

func stubCandidates(names ...string) []ai.Candidate {
	out := make([]ai.Candidate, 0, len(names))
	for _, name := range names {
		out = append(out, ai.Candidate{
			Name:           name,
			ScientificName: "Saraca asoca",
			WhyItMatches:   "It suits the courtyard you described.",
			Confidence:     ai.Confidence{Level: ai.ConfidenceModerate, Definition: ai.ConfidenceDefinitions[ai.ConfidenceModerate]},
			Identity:       ai.Identity{Status: ai.IdentityConfirmed, Source: "https://www.gbif.org/species/1"},
		})
	}
	return out
}

// postDiscovery sends one request through the real route table, so a test
// covers routing, validation, the handler and the response shape together.
func postDiscovery(t *testing.T, sug discovery.Suggester, res research.Provider, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	api := &API{
		mux: http.NewServeMux(),
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if sug != nil {
		api.pipeline = &discovery.Pipeline{
			Suggester: sug,
			Research:  res,
			Log:       api.log,
		}
	}
	api.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/discovery", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.mux.ServeHTTP(w, r)

	var payload map[string]any
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &payload)
	}
	return w, payload
}

// TestDiscoveryAcceptsAFreeTextIntention is the point of the endpoint. A
// visitor asking about a tradition Everroot has never heard of must reach the
// model, not a 400 that says the question is unsupported.
func TestDiscoveryAcceptsAFreeTextIntention(t *testing.T) {
	sug := &stubDiscoverer{result: ai.DiscoverResult{Candidates: stubCandidates("Ashoka")}}
	w, payload := postDiscovery(t, sug, nil, `{"intention":"a tree that means something to the Ahom people"}`)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s; an unfamiliar intention must not be refused", w.Code, w.Body)
	}
	if sug.got.Intention != "a tree that means something to the Ahom people" {
		t.Errorf("the model was told intention %q, want the visitor's own words", sug.got.Intention)
	}
	if payload["status"] != aiDiscoveryStatusOK {
		t.Errorf("status = %v", payload["status"])
	}
}

// TestDiscoveryMapsAKnownIntentionKeyOntoItsLabel keeps the interface's own
// vocabulary working exactly as it did.
func TestDiscoveryMapsAKnownIntentionKeyOntoItsLabel(t *testing.T) {
	sug := &stubDiscoverer{result: ai.DiscoverResult{Candidates: stubCandidates("Ashoka")}}
	w, _ := postDiscovery(t, sug, nil, `{"intention":"remember"}`)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if got := sug.got.Intention; got != "someone they want to remember" {
		t.Errorf("intention = %q, want the label for the remember key", got)
	}
}

func TestDiscoveryRequiresAnIntention(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"intention":""}`,
		`{"intention":"   "}`,
		`{"intention":"ab"}`, // below the minimum
	} {
		w, _ := postDiscovery(t, &stubDiscoverer{}, nil, body)
		if w.Code != 400 {
			t.Errorf("body %s returned %d, want 400", body, w.Code)
		}
	}
}

// TestDiscoveryRejectsAnOverlongIntention: the bound is about abuse, so it
// applies to free text and to the interface's own keys alike.
func TestDiscoveryRejectsAnOverlongIntention(t *testing.T) {
	long := strings.Repeat("tree ", 200)
	w, _ := postDiscovery(t, &stubDiscoverer{}, nil, `{"intention":"`+long+`"}`)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400 for an overlong intention", w.Code)
	}
}

// TestDiscoveryReportsAProviderFailureAsAnError is the requirement that an
// inference failure never reaches a visitor as an empty list.
func TestDiscoveryReportsAProviderFailureAsAnError(t *testing.T) {
	w, payload := postDiscovery(t, &stubDiscoverer{err: ai.ErrRateLimited}, nil, `{"intention":"remembering someone"}`)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if _, hasCandidates := payload["candidates"]; hasCandidates {
		t.Error("an error reply must not carry a candidates array")
	}
	if msg, _ := payload["error"].(string); !strings.Contains(strings.ToLower(msg), "busy") {
		t.Errorf("error = %v, want an actionable message", payload["error"])
	}
}

func TestDiscoveryReportsAnUnconfiguredModel(t *testing.T) {
	w, _ := postDiscovery(t, nil, nil, `{"intention":"remembering someone"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when no model is configured", w.Code)
	}
}

// TestDiscoverySaysSoWhenThereAreNoCandidates: a model that found nothing is
// an answer, and is labelled rather than presented as a success with a hole
// in it.
func TestDiscoverySaysSoWhenThereAreNoCandidates(t *testing.T) {
	w, payload := postDiscovery(t, &stubDiscoverer{result: ai.DiscoverResult{}}, nil, `{"intention":"curiosity"}`)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if payload["status"] != aiDiscoveryStatusNoCandidates {
		t.Errorf("status = %v, want %q", payload["status"], aiDiscoveryStatusNoCandidates)
	}
	if candidates, ok := payload["candidates"].([]any); !ok || len(candidates) != 0 {
		t.Errorf("candidates = %v, want an empty array rather than null", payload["candidates"])
	}
}

// TestDiscoveryCarriesSourcesSoEveryCitationResolves is requirement 5 and 6
// of the brief: a visitor must be able to follow a citation to a real address,
// and the addresses come from retrieval rather than from the model.
func TestDiscoveryCarriesSourcesSoEveryCitationResolves(t *testing.T) {
	sources := []research.Source{{
		ID:        "s1",
		Title:     "Ashoka",
		URL:       "https://en.wikipedia.org/wiki/Ashoka_tree",
		Publisher: "Wikipedia",
		Snippet:   "An evergreen tree.",
	}}
	candidates := stubCandidates("Ashoka")
	candidates[0].SourceIDs = []string{"s1"}

	w, payload := postDiscovery(t,
		&stubDiscoverer{result: ai.DiscoverResult{Candidates: candidates}},
		&stubResearcher{sources: sources},
		`{"intention":"a tree for a festival"}`)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	got, _ := payload["sources"].([]any)
	if len(got) != 1 {
		t.Fatalf("sources = %v, want the retrieved source", payload["sources"])
	}
	source := got[0].(map[string]any)
	if source["url"] != "https://en.wikipedia.org/wiki/Ashoka_tree" {
		t.Errorf("url = %v, want the retrieved address", source["url"])
	}
	if source["id"] != "s1" {
		t.Errorf("id = %v, want the ID the candidate cites", source["id"])
	}
	if source["retrievedAt"] == nil || source["retrievedAt"] == "" {
		t.Error("a source must carry a retrieval time")
	}
}

// TestDiscoveryNeverReportsAResearchOutageAsSuccess: when research failed, the
// reply says so, and the model is told so too.
func TestDiscoveryNeverReportsAResearchOutageAsSuccess(t *testing.T) {
	sug := &stubDiscoverer{result: ai.DiscoverResult{Candidates: stubCandidates("Ashoka")}}
	w, payload := postDiscovery(t, sug, &stubResearcher{err: research.ErrUnavailable}, `{"intention":"curiosity"}`)

	if w.Code != 200 {
		t.Fatalf("status = %d; a research outage must not fail the discovery", w.Code)
	}
	if payload["researchStatus"] != discovery.StatusUnavailable {
		t.Errorf("researchStatus = %v, want %q", payload["researchStatus"], discovery.StatusUnavailable)
	}
	summary, _ := payload["researchSummary"].(string)
	if !strings.Contains(summary, "Not researched") {
		t.Errorf("researchSummary = %q, must say the answer was not researched", summary)
	}
	if sug.got.ResearchStatus != ai.ResearchStatusUnavailable {
		t.Errorf("the model was told %q, want %q so it cannot cite a source it never saw",
			sug.got.ResearchStatus, ai.ResearchStatusUnavailable)
	}
}

func TestDiscoveryPassesTheVisitorsContextThrough(t *testing.T) {
	sug := &stubDiscoverer{result: ai.DiscoverResult{Candidates: stubCandidates("Ashoka")}}
	w, _ := postDiscovery(t, sug, nil, `{
		"intention":"culture",
		"description":"something my mother recognised",
		"tradition":"Tamil",
		"region":"Tamil Nadu",
		"nakshatra":"Ashwini",
		"constraints":["a small courtyard","full sun"],
		"limit":2
	}`)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if sug.got.Description != "something my mother recognised" {
		t.Errorf("description = %q", sug.got.Description)
	}
	if sug.got.Tradition != "Tamil" || sug.got.Region != "Tamil Nadu" || sug.got.Nakshatra != "Ashwini" {
		t.Errorf("context = %q/%q/%q", sug.got.Tradition, sug.got.Region, sug.got.Nakshatra)
	}
	if len(sug.got.Constraints) != 2 {
		t.Errorf("constraints = %v", sug.got.Constraints)
	}
	if sug.got.Count != 2 {
		t.Errorf("count = %d, want 2", sug.got.Count)
	}
}

func TestDiscoveryRejectsAnInvalidLimitAndOversizedLists(t *testing.T) {
	cases := map[string]string{
		"limit too high": `{"intention":"curiosity","limit":99}`,
		"limit zero-ish": `{"intention":"curiosity","limit":-1}`,
		"too many constraints": `{"intention":"curiosity","constraints":[` +
			strings.Repeat(`"x",`, 25) + `"x"]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			w, _ := postDiscovery(t, &stubDiscoverer{}, nil, body)
			if w.Code != 400 {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

// TestDiscoveryPublishesWhatConfidenceMeans: the reply must carry the
// definitions, so the interface can explain a level rather than implying a
// measured score.
func TestDiscoveryPublishesWhatConfidenceMeans(t *testing.T) {
	w, payload := postDiscovery(t, &stubDiscoverer{result: ai.DiscoverResult{Candidates: stubCandidates("Ashoka")}}, nil, `{"intention":"curiosity"}`)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	definitions, ok := payload["confidenceDefinitions"].(map[string]any)
	if !ok {
		t.Fatalf("confidenceDefinitions = %T, want an object", payload["confidenceDefinitions"])
	}
	for _, level := range []string{ai.ConfidenceHigh, ai.ConfidenceModerate, ai.ConfidenceLow, ai.ConfidenceInsufficient} {
		if definitions[level] == nil || definitions[level] == "" {
			t.Errorf("no published definition for level %q", level)
		}
	}
}

// TestResolveIntention pins the rule that decides whether a question is
// attempted at all.
func TestResolveIntention(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		valid bool
	}{
		{"remember", "someone they want to remember", true},
		{"  remember  ", "someone they want to remember", true},
		{"a tree for the Ahom", "a tree for the Ahom", true},
		{"", "", false},
		{"   ", "", false},
		{"ab", "", false},
		{strings.Repeat("a", 301), "", false},
	}
	for _, tc := range cases {
		got, ok := resolveIntention(tc.in)
		if ok != tc.valid {
			t.Errorf("resolveIntention(%q) ok = %v, want %v", tc.in, ok, tc.valid)
		}
		if got != tc.want {
			t.Errorf("resolveIntention(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
