package conversation

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"everroot/internal/ai"
)

// The tests here run with no database and no provider. That is the point:
// nothing in this flow is supposed to need either to produce an answer, and a
// test that quietly borrowed one would stop proving it.

// stubRecommender stands in for the model. It records what it was asked so the
// request itself can be asserted on — that is where most regressions in this
// flow would actually show up.
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

// stubExtractor stands in for the detail-extraction call, so a test can say
// what a visitor's sentence was understood to contain without spending quota.
type stubExtractor struct{ details extractedDetails }

func (s *stubExtractor) ChatCompletion(_ context.Context, _ []ai.Message) (string, error) {
	return `{"tradition":"` + s.details.Tradition + `","region":"` + s.details.Region +
		`","nakshatra":"` + s.details.Nakshatra + `"}`, nil
}

// stubGate is the reviewed publication gate. Its verdict must never decide
// whether a visitor is helped, only how the reply is labelled.
type stubGate struct{ verdict string }

func (s *stubGate) AssociationVerdict(_ context.Context, _, _, _ string) string {
	return s.verdict
}

func suggestions(names ...string) ai.SuggestResult {
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

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func manager(rec Recommender, gate CulturalGate) *Manager {
	// No pool and no AI client: the database and the provider are both absent
	// on purpose.
	return New(nil, nil, rec, gate, logger())
}

func sessionIn(state, intent string) *Session {
	return &Session{ID: "sess-test", State: state, Intent: intent}
}

// --- 1. A conversation reaches suggestions with no database and no catalogue ---

func TestRecommendationsNeedNeitherDatabaseNorCatalogue(t *testing.T) {
	rec := &stubRecommender{result: suggestions("Neem", "Peepal")}
	m := manager(rec, nil) // db nil, gate nil, catalogue not even in this package

	got, err := m.generateRecommendations(context.Background(), sessionIn(StateCollectingDetails, IntentCurious), "a tree for a small courtyard")
	if err != nil {
		t.Fatalf("generateRecommendations: %v", err)
	}
	if got.State != StateDiscoveryComplete {
		t.Errorf("state = %q, want %q", got.State, StateDiscoveryComplete)
	}
	if got.Recommendations == nil || len(got.Recommendations.Suggestions) != 2 {
		t.Fatalf("recommendations = %+v, want 2 suggestions", got.Recommendations)
	}
	if rec.calls != 1 {
		t.Errorf("recommender calls = %d, want 1", rec.calls)
	}
	if got.Recommendations.PromptVersion == "" || got.Recommendations.Disclaimer == "" {
		t.Error("provenance must travel with the suggestions, not be dropped")
	}
}

// --- 2. An unfamiliar tradition is context, not a rejection ---

func TestUnfamiliarTraditionStillGetsSuggestions(t *testing.T) {
	for _, tc := range []struct {
		name string
		sess *Session
	}{
		{"tradition Everroot has never heard of", &Session{ID: "s", State: StateCollectingDetails, Intent: IntentExplore, TraditionSlug: "Sikh", Nakshatra: "Ashwini"}},
		{"Nakshatra with no registry record", &Session{ID: "s", State: StateCollectingDetails, Intent: IntentExplore, Nakshatra: "Moola"}},
		{"nothing gathered at all", &Session{ID: "s", State: StateCollectingDetails, Intent: IntentCurious}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &stubRecommender{result: suggestions("Neem")}
			m := manager(rec, &stubGate{verdict: AssocNoReviewedMatch})

			got, err := m.generateRecommendations(context.Background(), tc.sess, "my grandfather always spoke of this tree")
			if err != nil {
				t.Fatalf("generateRecommendations: %v", err)
			}
			if got.State != StateDiscoveryComplete {
				t.Errorf("state = %q, want %q — an unregistered tradition must not be a refusal",
					got.State, StateDiscoveryComplete)
			}
			if len(got.Recommendations.Suggestions) == 0 {
				t.Error("no suggestions returned for an unregistered tradition")
			}
		})
	}
}

// --- 3. An absent reviewed mapping annotates; it does not block ---

func TestNoReviewedMappingDoesNotBlockSuccess(t *testing.T) {
	rec := &stubRecommender{result: suggestions("Neem")}
	m := manager(rec, &stubGate{verdict: AssocNoReviewedMatch})

	got, err := m.generateRecommendations(context.Background(),
		sessionIn(StateCollectingDetails, IntentRemember), "for my grandmother")
	if err != nil {
		t.Fatalf("generateRecommendations: %v", err)
	}
	if got.State != StateDiscoveryComplete {
		t.Errorf("state = %q, want %q: a missing reviewed mapping must not end the conversation",
			got.State, StateDiscoveryComplete)
	}
	if got.Recommendations.Evidence.CulturalAssociation != AssocNoReviewedMatch {
		t.Errorf("culturalAssociation = %q, want %q",
			got.Recommendations.Evidence.CulturalAssociation, AssocNoReviewedMatch)
	}
}

func TestStateNoVerifiedMappingIsNeverProduced(t *testing.T) {
	// The old flow entered this state whenever the registry had no record. It
	// must not come back through any path that is not a genuine refusal.
	for _, gate := range []string{AssocNoReviewedMatch, AssocNotQueryable, AssocReviewedMatch, ""} {
		rec := &stubRecommender{result: suggestions("Neem")}
		m := manager(rec, &stubGate{verdict: gate})
		got, err := m.generateRecommendations(context.Background(),
			sessionIn(StateCollectingDetails, IntentExplore), "anything under the sun")
		if err != nil {
			t.Fatalf("gate %q: %v", gate, err)
		}
		if got.State == StateNoVerifiedMapping {
			t.Errorf("gate %q produced state %q, which the AI-first flow must not use",
				gate, StateNoVerifiedMapping)
		}
	}
}

// --- 4. A machine-written suggestion is never presented as a reviewed fact ---

func TestSuggestionsAreNeverLabelledAsReviewedCulturalFacts(t *testing.T) {
	rec := &stubRecommender{result: suggestions("Neem")}
	m := manager(rec, &stubGate{verdict: AssocNoReviewedMatch})

	got, err := m.generateRecommendations(context.Background(),
		sessionIn(StateCollectingDetails, IntentExplore), "trees of my tradition")
	if err != nil {
		t.Fatalf("generateRecommendations: %v", err)
	}
	e := got.Recommendations.Evidence
	if e.CulturalAssociation == AssocReviewedMatch {
		t.Error("culturalAssociation claims a reviewed match when the gate said there was none")
	}
	if e.BotanicalIdentity == "authority_verified" {
		t.Error("botanicalIdentity claims authority verification this flow never asked for")
	}
	if !strings.Contains(got.Recommendations.Disclaimer, "Machine-written") {
		t.Errorf("disclaimer %q does not identify the suggestions as machine-written",
			got.Recommendations.Disclaimer)
	}
	// The verdict the model was told is the verdict the visitor reads.
	if rec.got.AssociationStatus != AssocNoReviewedMatch {
		t.Errorf("prompt AssociationStatus = %q, want %q — the two must not disagree",
			rec.got.AssociationStatus, AssocNoReviewedMatch)
	}
}

func TestUnqueryableGateIsReportedAsUnqueryable(t *testing.T) {
	rec := &stubRecommender{result: suggestions("Neem")}
	m := manager(rec, nil) // no gate at all: nothing was checked

	if _, err := m.generateRecommendations(context.Background(),
		sessionIn(StateCollectingDetails, IntentExplore), "surprise me"); err != nil {
		t.Fatalf("generateRecommendations: %v", err)
	}
	if rec.got.AssociationStatus != AssocNotQueryable {
		t.Errorf("prompt AssociationStatus = %q, want %q: an unchecked verdict must not be guessed",
			rec.got.AssociationStatus, AssocNotQueryable)
	}
}

// --- 5. A broken or absent engine says so, instead of pretending ---

func TestUnconfiguredRecommenderIsAnHonestFailure(t *testing.T) {
	m := manager(nil, nil)

	got, err := m.generateRecommendations(context.Background(),
		sessionIn(StateCollectingDetails, IntentCurious), "anything you like")
	if err != nil {
		t.Fatalf("generateRecommendations: %v", err)
	}
	if got.State != StateRecommendationUnavailable {
		t.Errorf("state = %q, want %q", got.State, StateRecommendationUnavailable)
	}
	if got.Recommendations != nil {
		t.Error("an unconfigured engine must not return an empty success payload")
	}
	joined := strings.Join(got.Suggestions, " ")
	if !strings.Contains(joined, "Start over") || !strings.Contains(joined, "Try again") {
		t.Errorf("suggestions = %v, want a way onward rather than a dead end", got.Suggestions)
	}
}

func TestFailingProviderIsAnHonestFailure(t *testing.T) {
	m := manager(&stubRecommender{err: errors.New("provider unreachable")}, nil)

	got, err := m.generateRecommendations(context.Background(),
		sessionIn(StateCollectingDetails, IntentRemember), "someone I miss")
	if err != nil {
		t.Fatalf("generateRecommendations: %v", err)
	}
	if got.State != StateRecommendationUnavailable {
		t.Errorf("state = %q, want %q: a provider failure is not an absence of trees",
			got.State, StateRecommendationUnavailable)
	}
	if got.Recommendations != nil {
		t.Error("a provider failure must not surface as an empty success")
	}
	if strings.Contains(strings.ToLower(got.Message), "no trees") ||
		strings.Contains(strings.ToLower(got.Message), "didn't match") {
		t.Errorf("message %q presents a failure as a lack of matching trees", got.Message)
	}
}

func TestModelFindingNothingIsNotAServiceFailure(t *testing.T) {
	m := manager(&stubRecommender{result: ai.SuggestResult{
		PromptVersion: ai.RecommendPromptVersion, Model: "stub-model", Disclaimer: ai.SuggestionDisclaimer,
	}}, nil)

	got, err := m.generateRecommendations(context.Background(),
		sessionIn(StateCollectingDetails, IntentCurious), "whatever you think")
	if err != nil {
		t.Fatalf("generateRecommendations: %v", err)
	}
	if got.State != StateNoSuggestions {
		t.Errorf("state = %q, want %q", got.State, StateNoSuggestions)
	}
	if got.State == StateRecommendationUnavailable {
		t.Error("an empty answer from the model is not a service outage")
	}
	if got.Recommendations == nil || got.Recommendations.Status != recommendStatusNoSuggestions {
		t.Errorf("status = %+v, want %q", got.Recommendations, recommendStatusNoSuggestions)
	}
	if len(got.Recommendations.Suggestions) != 0 {
		t.Errorf("suggestions = %v, want none", got.Recommendations.Suggestions)
	}
}

// --- 6. The hardcoded loopback URL is gone ---

func TestFlowNoLongerCallsItselfOverTheLoopbackInterface(t *testing.T) {
	src, err := os.ReadFile("conversation.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	body := string(src)
	for _, banned := range []string{
		"127.0.0.1:8080",
		"localhost:8080",
		"api/v1/discoveries",
		"\"net/http\"",
		"http.NewRequest",
		"http.Client",
		"callDiscovery",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("conversation.go still references %q; the flow must not call the HTTP API back", banned)
		}
	}
}

// --- 7. The existing conversational experience still works ---

func TestGreetsThenOffersIntention(t *testing.T) {
	m := manager(&stubRecommender{}, nil)

	got, err := m.Handle(context.Background(), ConversationRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.State != StateGreeting {
		t.Errorf("state = %q, want %q", got.State, StateGreeting)
	}
	if len(got.Suggestions) == 0 {
		t.Error("greeting offers no intentions")
	}
	if got.Message == "" {
		t.Error("greeting has no message")
	}

	picked, err := m.Handle(context.Background(), ConversationRequest{Intent: IntentRemember})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if picked.State != StateIntentSelected {
		t.Errorf("state = %q, want %q", picked.State, StateIntentSelected)
	}
	if picked.Intent != IntentRemember {
		t.Errorf("intent = %q, want %q", picked.Intent, IntentRemember)
	}
	if picked.Message == "" || len(picked.Suggestions) == 0 {
		t.Error("an acknowledged intention must offer the next step")
	}
}

func TestUnknownIntentionIsNotAccepted(t *testing.T) {
	m := manager(&stubRecommender{}, nil)
	got, err := m.Handle(context.Background(), ConversationRequest{Intent: "definitely-not-an-intent"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.State != StateGreeting {
		t.Errorf("state = %q, want %q: an unknown intention re-prompts rather than advancing",
			got.State, StateGreeting)
	}
}

func TestSessionIDIsAlwaysReturned(t *testing.T) {
	m := manager(&stubRecommender{}, nil)
	got, err := m.Handle(context.Background(), ConversationRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.SessionID == "" {
		t.Error("session_id is empty; the frontend cannot link to results without it")
	}
}

func TestRestartClearsEverythingGathered(t *testing.T) {
	m := manager(&stubRecommender{}, nil)
	s := sessionIn(StateDiscoveryComplete, IntentRemember)
	s.TraditionSlug = "nakshatra-tree-association"
	s.RegionName = "Tamil Nadu"
	s.Nakshatra = "Ashwini"
	s.DiscoveryMappingID = strPtr("map-1")
	s.DiscoveryRevisionID = strPtr("rev-1")

	got, err := m.restart(context.Background(), s)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if got.State != StateGreeting {
		t.Errorf("state = %q, want %q", got.State, StateGreeting)
	}
	if s.Intent != "" || s.TraditionSlug != "" || s.RegionName != "" || s.Nakshatra != "" {
		t.Errorf("restart left gathered detail behind: %+v", s)
	}
	if s.DiscoveryMappingID != nil || s.DiscoveryRevisionID != nil {
		t.Error("restart left a stale reviewed mapping on the session")
	}
	if s.State != StateGreeting {
		t.Errorf("session state = %q, want %q", s.State, StateGreeting)
	}
}

func TestFinishedOrFailedSessionsOfferAWayBack(t *testing.T) {
	// Every terminal state must let the visitor start again, including the two
	// the AI-first flow introduced.
	for _, state := range []string{
		StateDiscoveryComplete, StateNoSuggestions, StateRecommendationUnavailable,
	} {
		m := manager(&stubRecommender{}, nil)
		got, err := m.Handle(context.Background(), ConversationRequest{SessionID: "x"})
		if err != nil {
			t.Fatalf("state %q: %v", state, err)
		}
		// With no database the session is always fresh, so drive restart
		// directly to prove the mapping.
		_ = got
		s := sessionIn(state, IntentCurious)
		re, err := m.restart(context.Background(), s)
		if err != nil {
			t.Fatalf("restart from %q: %v", state, err)
		}
		if re.State != StateGreeting {
			t.Errorf("restart from %q landed in %q, want %q", state, re.State, StateGreeting)
		}
	}
}

// --- The visitor's own words reach the prompt, and only their words ---

func TestAnswerCarriesTheVisitorsWords(t *testing.T) {
	rec := &stubRecommender{result: suggestions("Neem")}
	m := manager(rec, nil)
	s := sessionIn(StateCollectingDetails, IntentRemember)
	s.TraditionSlug = "nakshatra-tree-association"
	s.Nakshatra = "Ashwini"

	if _, err := m.generateRecommendations(context.Background(), s, "  she planted a neem by the well  "); err != nil {
		t.Fatalf("generateRecommendations: %v", err)
	}
	if !strings.Contains(rec.got.Answer, "she planted a neem by the well") {
		t.Errorf("Answer = %q, missing the visitor's own words", rec.got.Answer)
	}
	if !strings.Contains(rec.got.Answer, "Ashwini") {
		t.Errorf("Answer = %q, missing gathered context", rec.got.Answer)
	}
}

func TestAnswerIsBoundedByThePromptLimit(t *testing.T) {
	long := strings.Repeat("a very long sentence about a tree. ", 200)
	s := sessionIn(StateCollectingDetails, IntentCurious)
	got := answerFor(s, long)
	if len(got) > ai.MaxPromptAnswerChars {
		t.Errorf("answer is %d bytes, over the %d-byte prompt limit", len(got), ai.MaxPromptAnswerChars)
	}
}

func TestEveryConversationIntentMapsToARecommendationIntention(t *testing.T) {
	for _, intent := range []string{IntentCelebrate, IntentRemember, IntentExplore, IntentGrow, IntentCurious} {
		id, label := intentionFor(intent)
		if id == "" || label == "" {
			t.Errorf("intentionFor(%q) = (%q, %q); both halves are needed", intent, id, label)
		}
	}
	if id, _ := intentionFor("not-an-intent"); id != "" {
		t.Errorf("intentionFor of an unknown intent returned %q, want empty", id)
	}
}

func TestIntentionMappingIsStable(t *testing.T) {
	// These identifiers are the recommendation endpoint's vocabulary. Changing
	// one silently would stop a conversation and a form from meaning the same
	// thing by the same word.
	want := map[string]string{
		IntentCelebrate: "personal",
		IntentRemember:  "remember",
		IntentExplore:   "culture",
		IntentGrow:      "future",
		IntentCurious:   "explore",
	}
	for intent, id := range want {
		got, _ := intentionFor(intent)
		if got != id {
			t.Errorf("intentionFor(%q) = %q, want %q", intent, got, id)
		}
	}
}

func TestUnfamiliarTraditionIsNotCollapsedToARegisteredOne(t *testing.T) {
	// A tradition Everroot carries is slugged for the registry's benefit; one
	// it does not carry must survive intact, because that is the only way the
	// model can be told what the visitor actually said.
	if got := traditionNameToSlug("Hindu tradition"); got == "" {
		t.Error("a familiar tradition lost its slug")
	}
	if got := traditionNameToSlug("Sikh"); got != "Sikh" {
		t.Errorf("traditionNameToSlug(\"Sikh\") = %q, want it passed through unchanged", got)
	}
	if got := traditionNameToSlug("  Bahá'í  "); got != "Bahá'í" {
		t.Errorf("traditionNameToSlug trimmed incorrectly: %q", got)
	}
}

// TestDetailTurnFoldsWhatTheExtractorUnderstood covers the gathering half of
// the flow. Extraction itself belongs to Handle; what matters here is that
// whatever was understood is carried onto the session, and that a field the
// extractor could not fill does not erase one given on an earlier turn.
func TestDetailTurnFoldsWhatTheExtractorUnderstood(t *testing.T) {
	s := sessionIn(StateCollectingDetails, IntentExplore)
	s.TraditionSlug = "nakshatra-tree-association"

	applyDetails(s, extractedDetails{Tradition: "Tamil tradition", Nakshatra: "Rohini"})

	if s.Nakshatra != "Rohini" {
		t.Errorf("nakshatra = %q, want %q", s.Nakshatra, "Rohini")
	}
	if s.TraditionSlug == "" {
		t.Error("tradition was not carried onto the session")
	}

	// A later turn mentioning only a region must keep the Nakshatra.
	before := s.Nakshatra
	applyDetails(s, extractedDetails{Region: "Tamil Nadu"})
	if s.Nakshatra != before {
		t.Errorf("nakshatra changed from %q to %q on a turn that never mentioned it", before, s.Nakshatra)
	}
	if s.RegionName != "Tamil Nadu" {
		t.Errorf("region = %q, want %q", s.RegionName, "Tamil Nadu")
	}
}

// TestDetailGatheringNeverStallsTheVisitor proves the question is an offer and
// not a toll gate: whoever says they do not know goes straight through.
func TestDetailGatheringNeverStallsTheVisitor(t *testing.T) {
	for _, msg := range []string{
		"I don't know", "no idea", "not sure", "just suggest something",
		"surprise me", "whatever you like", "I don't have a nakshatra",
	} {
		m := manager(&stubRecommender{result: suggestions("Neem")}, nil)
		got, err := m.Handle(context.Background(), ConversationRequest{})
		if err != nil {
			t.Fatalf("greeting: %v", err)
		}
		_ = got

		s := sessionIn(StateIntentSelected, IntentCurious)
		if !m.enoughContext(s, msg) {
			t.Errorf("%q should have gone straight to suggestions", msg)
		}
	}
}

// TestARealAnswerStillGetsOneQuestionOfContext keeps the multi-turn experience:
// a visitor who shared something is asked once for the rest, not waved through.
func TestARealAnswerStillGetsOneQuestionOfContext(t *testing.T) {
	m := manager(&stubRecommender{}, nil)
	s := sessionIn(StateIntentSelected, IntentExplore)
	if m.enoughContext(s, "I have been thinking about the trees in my grandmother's courtyard") {
		t.Error("a substantive answer was waved through without the follow-up question")
	}
	s.TraditionSlug = "Sikh"
	s.Nakshatra = "Ashwini"
	if !m.enoughContext(s, "Sikh tradition, Ashwini") {
		t.Error("a visitor who gave both a tradition and a Nakshatra should not be asked again")
	}
}

func strPtr(s string) *string { return &s }
