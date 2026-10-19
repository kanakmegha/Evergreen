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
)

// stubRecommender stands in for the configured model so these tests spend no
// quota and never reach a network. It records what the handler asked for, which
// is how the request schema itself gets tested.
type stubRecommender struct {
	result ai.SuggestResult
	err    error
	got    ai.RecommendInput
	calls  int
}

func (s *stubRecommender) SuggestTrees(_ context.Context, in ai.RecommendInput) (ai.SuggestResult, error) {
	s.calls++
	s.got = in
	return s.result, s.err
}

func stubSuggestions(names ...string) ai.SuggestResult {
	result := ai.SuggestResult{
		PromptVersion: ai.RecommendPromptVersion,
		Model:         "stub-model",
		Disclaimer:    ai.SuggestionDisclaimer,
	}
	for _, name := range names {
		result.Suggestions = append(result.Suggestions, ai.Suggestion{
			Name:            name,
			ScientificName:  "Azadirachta indica",
			WhyItMatches:    "It fits the courtyard you described.",
			BotanicalDetail: "A tree in the mahogany family.",
			Growing:         []string{"full sun"},
		})
	}
	return result
}

// postRecommendations sends one request through the real route table, so the
// test covers routing, validation, the handler and the response shape together.
func postRecommendations(t *testing.T, rec recommender, body string, logw io.Writer) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	api := &API{
		mux:         http.NewServeMux(),
		log:         slog.New(slog.NewTextHandler(logw, nil)),
		recommender: rec,
	}
	api.routes()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.mux.ServeHTTP(w, req)
	decoded := map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, w.Body.String())
	}
	return w, decoded
}

func suggestionsOf(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["suggestions"].([]any)
	if !ok {
		t.Fatalf("no suggestions array in %v", body)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(map[string]any))
	}
	return out
}

func evidenceOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	evidence, ok := body["evidence"].(map[string]any)
	if !ok {
		t.Fatalf("no evidence in %v", body)
	}
	return evidence
}

// The core product rule: any intention can produce a suggestion, with no
// database and no botanical catalogue behind it. A harvested intention is
// never a precondition for an answer, and the words the visitor wrote travel
// into the request that builds the prompt.
func TestRecommendationsServeAnyIntentionWithoutADatabaseOrCatalogue(t *testing.T) {
	stub := &stubRecommender{result: stubSuggestions("Neem", "Jamun")}
	var logs bytes.Buffer
	w, body := postRecommendations(t, stub,
		`{"intention":"personal","answer":"A tree that reminds me of the monsoons at home.","picks":["shade"]}`,
		&logs)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
	if got := suggestionsOf(t, body); len(got) != 2 || got[0]["name"] != "Neem" {
		t.Fatalf("suggestions = %v", got)
	}
	if body["promptVersion"] != ai.RecommendPromptVersion {
		t.Fatalf("promptVersion = %v", body["promptVersion"])
	}
	if body["disclaimer"] == "" {
		t.Fatal("machine-written suggestions must travel with a disclaimer")
	}
	// Nothing was checked against the publication gate, so nothing may claim
	// anything about culture — least of all a match.
	if got := evidenceOf(t, body)["culturalAssociation"]; got != "not_queryable" {
		t.Fatalf("culturalAssociation = %v, want not_queryable", got)
	}
	if got := evidenceOf(t, body)["botanicalIdentity"]; got != "unavailable" {
		t.Fatalf("botanicalIdentity = %v: a model's prose is not an authority record", got)
	}
	if got := evidenceOf(t, body)["documentedStories"]; got != "no_reviewed_record" {
		t.Fatalf("documentedStories = %v", got)
	}

	if stub.got.IntentionID != "personal" || stub.got.IntentionLabel == "" {
		t.Fatalf("intention passed through as %+v", stub.got)
	}
	if stub.got.Answer != "A tree that reminds me of the monsoons at home." {
		t.Fatalf("the visitor's answer was not carried: %q", stub.got.Answer)
	}
	if stub.got.Count != ai.DefaultSuggestions {
		t.Fatalf("count = %d", stub.got.Count)
	}
	if len(stub.got.Picks) != 1 {
		t.Fatalf("picks = %v", stub.got.Picks)
	}
}

// A cultural selection whose records do not cover it still gets a suggestion,
// and the reply says the association is not verified rather than inventing one.
func TestRecommendationsForCulturalSelectionWithoutVerifiedMapping(t *testing.T) {
	stub := &stubRecommender{result: stubSuggestions("Peepal")}
	var logs bytes.Buffer
	w, body := postRecommendations(t, stub,
		`{"intention":"culture","choice":"A Nakshatra tree association","tradition":"nakshatra-tree-association","nakshatra":"Rohini","answer":"I want to understand what is recorded."}`,
		&logs)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := evidenceOf(t, body)["culturalAssociation"]; got != "no_reviewed_match" {
		t.Fatalf("culturalAssociation = %v, want no_reviewed_match", got)
	}
	if got := suggestionsOf(t, body); len(got) != 1 {
		t.Fatalf("a cultural selection with no mapping must still get a suggestion: %v", got)
	}
	if stub.got.Tradition != "nakshatra-tree-association" || stub.got.Nakshatra != "Rohini" {
		t.Fatalf("selection passed through as %+v", stub.got)
	}
}

func TestRecommendationsRejectUnsupportedIntention(t *testing.T) {
	for _, body := range []string{
		`{"intention":"make-money","answer":"a tree"}`,
		`{"intention":"","answer":"a tree"}`,
	} {
		w, decoded := postRecommendations(t, &stubRecommender{result: stubSuggestions("Neem")}, body, io.Discard)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", body, w.Code)
		}
		if decoded["error"] != "unsupported intention" {
			t.Errorf("%s: error = %v", body, decoded["error"])
		}
	}
}

func TestRecommendationsValidateTheRequest(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"answer too long": {
			body: `{"intention":"personal","answer":"` + strings.Repeat("a", 701) + `"}`,
			want: "answer is too long",
		},
		"selection too long": {
			body: `{"intention":"personal","nakshatra":"` + strings.Repeat("n", 101) + `"}`,
			want: "invalid selection",
		},
		"limit out of range": {
			body: `{"intention":"personal","limit":9}`,
			want: "limit must be between 1 and 5",
		},
		"too many picks": {
			body: `{"intention":"personal","picks":[` + strings.Repeat(`"x",`, 20) + `"x"]}`,
			want: "too many choices",
		},
	} {
		w, decoded := postRecommendations(t, &stubRecommender{result: stubSuggestions("Neem")}, tc.body, io.Discard)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, body = %s", name, w.Code, w.Body.String())
			continue
		}
		message, _ := decoded["error"].(string)
		if !strings.Contains(message, tc.want) {
			t.Errorf("%s: error = %q, want it to mention %q", name, message, tc.want)
		}
	}
}

// An unconfigured deployment is an operator fix, so it says so instead of
// pretending the visitor's answers found nothing.
func TestRecommendationsUnconfiguredAIIsReportedAsSuch(t *testing.T) {
	w, decoded := postRecommendations(t, nil, `{"intention":"personal","answer":"anything"}`, io.Discard)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	message, _ := decoded["error"].(string)
	if !strings.Contains(message, "recommendation") {
		t.Fatalf("error must name the capability that is unavailable: %q", message)
	}
	if !strings.Contains(message, "HF_TOKEN") {
		t.Fatalf("error must say what fixes it: %q", message)
	}
	if _, ok := decoded["suggestions"]; ok {
		t.Fatal("a failure must never arrive dressed as a successful empty answer")
	}
}

// A provider that is busy is a retry, not an absence of trees.
func TestRecommendationsRateLimitedProviderIsRetriable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"Model busy, retry later","code":"engine_overloaded"}}`)
	}))
	t.Cleanup(srv.Close)

	client := newStubAIClient(t, srv.URL)

	var logs bytes.Buffer
	w, decoded := postRecommendations(t, client, `{"intention":"personal","answer":"a courtyard tree"}`, &logs)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q", w.Header().Get("Retry-After"))
	}
	if _, ok := decoded["suggestions"]; ok {
		t.Fatal("a rate-limited provider must not be reported as an empty answer")
	}
	if !strings.Contains(logs.String(), "category=rate_limited") {
		t.Fatalf("failure not classified in the log: %s", logs.String())
	}
}

// Model output Everroot cannot read is a failure to report, never a 200 with
// nothing in it: the visitor must be able to tell "the model broke" from
// "Everroot has nothing for you".
func TestRecommendationsUnreadableModelOutputIsNotAnEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"cmpl-1","choices":[{"message":{"role":"assistant","content":"I am afraid I cannot help with that."}}]}`)
	}))
	t.Cleanup(srv.Close)

	client := newStubAIClient(t, srv.URL)

	w, decoded := postRecommendations(t, client, `{"intention":"personal","answer":"anything at all"}`, io.Discard)

	if w.Code == http.StatusOK {
		t.Fatalf("unreadable model output was reported as success: %s", w.Body.String())
	}
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 for a reply Everroot could not read", w.Code)
	}
	if _, ok := decoded["suggestions"]; ok {
		t.Fatal("a failure must never arrive as an empty suggestions array")
	}
}

// "The model found nothing it would stand behind" is a real answer and is
// labelled as one, so the interface can explain it instead of showing a blank.
func TestRecommendationsNoGenuineFitIsReportedExplicitly(t *testing.T) {
	stub := &stubRecommender{result: ai.SuggestResult{
		Suggestions:   []ai.Suggestion{},
		PromptVersion: ai.RecommendPromptVersion,
		Model:         "stub-model",
		Disclaimer:    ai.SuggestionDisclaimer,
	}}
	w, body := postRecommendations(t, stub, `{"intention":"explore","answer":"something I cannot describe"}`, io.Discard)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if body["status"] != "no_suggestions" {
		t.Fatalf("status = %v, want no_suggestions", body["status"])
	}
	if got := suggestionsOf(t, body); len(got) != 0 {
		t.Fatalf("suggestions = %v", got)
	}
}

func TestRecommendationsProviderFailureIsReported(t *testing.T) {
	stub := &stubRecommender{err: ai.ErrProvider}
	w, decoded := postRecommendations(t, stub, `{"intention":"personal","answer":"a tree"}`, io.Discard)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if _, ok := decoded["suggestions"]; ok {
		t.Fatal("a provider failure must not be reported as an empty answer")
	}
}

// The request carries what the visitor wrote about their own life. If the model
// fails, none of it may end up in the log — only the classification of what
// went wrong.
func TestRecommendationsFailureLogNeverCarriesVisitorWords(t *testing.T) {
	const privateWords = "my mother's name is Kalpana and the house is in Kozhikode"
	stub := &stubRecommender{err: ai.ErrTimeout}

	var logs bytes.Buffer
	w, _ := postRecommendations(t, stub,
		`{"intention":"remember","answer":"`+privateWords+`"}`, &logs)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	logged := logs.String()
	if !strings.Contains(logged, "category=timeout") {
		t.Fatalf("the failure was not classified: %s", logged)
	}
	for _, leaked := range []string{privateWords, "Kalpana", "Kozhikode", "remember"} {
		if strings.Contains(logged, leaked) {
			t.Fatalf("visitor words reached the log: %s", logged)
		}
	}
}

// newStubAIClient points a real client at a fake provider, so the diagnosis,
// the status mapping and the Retry-After handling are exercised end to end
// without spending a model call.
func newStubAIClient(t *testing.T, baseURL string) *ai.Client {
	t.Helper()
	t.Setenv("HF_TOKEN", "hf_test_token_not_real")
	t.Setenv("HF_MODEL_ID", "openai/gpt-oss-120b:test")
	t.Setenv("HF_BASE_URL", baseURL)
	client, err := ai.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("ai.New: %v", err)
	}
	return client
}

// The gate verdict is the model's only licence to mention cultural
// associations, so it is handed over only in the one case the prompt may
// restate. A selection nobody made, and a selection Everroot can answer, must
// both leave the model silent about culture.
func TestRecommendationsHandTheModelOnlyTheUnverifiedVerdict(t *testing.T) {
	cultural := &stubRecommender{result: stubSuggestions("Peepal")}
	w, _ := postRecommendations(t, cultural,
		`{"intention":"culture","choice":"A Nakshatra tree association","tradition":"nakshatra-tree-association","nakshatra":"Rohini","answer":"I want to understand what is recorded."}`,
		io.Discard)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if cultural.got.AssociationStatus != ai.AssociationStatusUnverified {
		t.Fatalf("AssociationStatus = %q, want %q", cultural.got.AssociationStatus, ai.AssociationStatusUnverified)
	}

	// "explore" shares nothing with a tradition or a Nakshatra, so there is no
	// cultural question to explain and the model must be told nothing.
	personal := &stubRecommender{result: stubSuggestions("Neem")}
	w, _ = postRecommendations(t, personal,
		`{"intention":"explore","answer":"a tree for a hot courtyard"}`, io.Discard)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if personal.got.AssociationStatus == ai.AssociationStatusUnverified {
		t.Fatalf("the model was handed a verdict for a selection nobody made: %q",
			personal.got.AssociationStatus)
	}
}

// The model's bounded extras reach the consumer intact. The card renders them
// and the OpenAPI contract names them, so a handler that dropped them would be
// promising something the browser never sees.
func TestRecommendationsCarryTheSuggestionExtras(t *testing.T) {
	one := stubSuggestions("Neem").Suggestions[0]
	one.MatureSize = "15-20 m"
	one.CulturalNote = "Everroot has no reviewed record for this tradition and Nakshatra."
	stub := &stubRecommender{result: ai.SuggestResult{
		Suggestions:   []ai.Suggestion{one},
		PromptVersion: ai.RecommendPromptVersion,
		Model:         "stub-model",
		Disclaimer:    ai.SuggestionDisclaimer,
	}}
	_, body := postRecommendations(t, stub,
		`{"intention":"culture","choice":"A Nakshatra tree association","tradition":"nakshatra-tree-association","nakshatra":"Rohini","answer":"I want to understand what is recorded."}`,
		io.Discard)

	got := suggestionsOf(t, body)
	if len(got) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(got))
	}
	if got[0]["matureSize"] != "15-20 m" {
		t.Errorf("matureSize = %v, want 15-20 m", got[0]["matureSize"])
	}
	if got[0]["culturalNote"] == "" {
		t.Errorf("culturalNote must reach the consumer, got %v", got[0])
	}
}

// A follow-up question travels with the suggestions, never instead of them, and
// is omitted rather than sent empty — a consumer should never have to
// distinguish "no question" from "no answer" by inspecting a string.
func TestRecommendationsCarryTheFollowUpQuestionOnlyWhenThereIsOne(t *testing.T) {
	asking := &stubRecommender{result: ai.SuggestResult{
		Suggestions:      stubSuggestions("Neem").Suggestions,
		FollowUpQuestion: "How much direct sun does the spot get?",
		PromptVersion:    ai.RecommendPromptVersion,
		Model:            "stub-model",
		Disclaimer:       ai.SuggestionDisclaimer,
	}}
	_, body := postRecommendations(t, asking,
		`{"intention":"explore","answer":"a tree for a hot courtyard"}`, io.Discard)
	if got := body["followUpQuestion"]; got != "How much direct sun does the spot get?" {
		t.Fatalf("followUpQuestion = %v", got)
	}
	if len(suggestionsOf(t, body)) != 1 {
		t.Fatalf("a question must arrive with an answer, not instead of one: %v", body)
	}

	silent := &stubRecommender{result: stubSuggestions("Neem")}
	_, body = postRecommendations(t, silent,
		`{"intention":"explore","answer":"a tree for a hot courtyard"}`, io.Discard)
	if value, present := body["followUpQuestion"]; present {
		t.Fatalf("an absent question must be omitted, got %v", value)
	}
}
