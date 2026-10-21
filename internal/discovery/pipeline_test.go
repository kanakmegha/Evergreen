package discovery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"everroot/internal/ai"
	"everroot/internal/research"
)

// fakeSuggester records the input it was given and returns a fixed reply, so a
// pipeline test can assert on exactly what the model was shown.
type fakeSuggester struct {
	result ai.DiscoverResult
	err    error
	got    ai.DiscoverInput
	calls  int
}

func (f *fakeSuggester) Discover(_ context.Context, in ai.DiscoverInput) (ai.DiscoverResult, error) {
	f.got = in
	f.calls++
	if f.err != nil {
		return ai.DiscoverResult{}, f.err
	}
	return f.result, nil
}

// fakeResearch is a research.Provider test double.
type fakeResearch struct {
	sources []research.Source
	err     error
	got     research.Query
}

func (f *fakeResearch) Name() string { return "fake" }
func (f *fakeResearch) Search(_ context.Context, q research.Query, _ int) ([]research.Source, error) {
	f.got = q
	return f.sources, f.err
}

// fakeVerifier records which names it was asked about.
type fakeVerifier struct {
	identity research.Identity
	err      error
	asked    []string
}

func (f *fakeVerifier) VerifyIdentity(_ context.Context, name string) (research.Identity, error) {
	f.asked = append(f.asked, name)
	return f.identity, f.err
}

func aSource(id, title string) research.Source {
	return research.Source{
		ID:        id,
		Title:     title,
		URL:       "https://example.org/" + id,
		Publisher: "Example",
		Kind:      research.KindWebPage,
		Snippet:   "A snippet about " + title,
	}
}

func okReply() ai.DiscoverResult {
	return ai.DiscoverResult{
		Candidates: []ai.Candidate{{
			Name:           "Ashoka",
			ScientificName: "Saraca asoca",
			WhyItMatches:   "You said you wanted a tree for a small courtyard.",
		}},
	}
}

// TestPipelineWorksWithNoDatabase is requirement 1 of the brief, stated as a
// property rather than a connection test: the Pipeline type has no field that
// could reach PostgreSQL, so there is no configuration of it in which a
// database matters. This test pins that by constructing the pipeline the way
// the HTTP handler does and running a full discovery.
func TestPipelineWorksWithNoDatabase(t *testing.T) {
	researcher := &fakeResearch{sources: []research.Source{aSource("x", "Ashoka")}}
	p := &Pipeline{
		Suggester: &fakeSuggester{result: okReply()},
		Research:  researcher,
	}

	outcome := p.Run(context.Background(), Request{Intention: "something to remember her by"})
	if outcome.Err != nil {
		t.Fatalf("Run: %v", outcome.Err)
	}
	if len(outcome.Result.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(outcome.Result.Candidates))
	}
	if outcome.Status != StatusResearched {
		t.Errorf("status = %q, want %q", outcome.Status, StatusResearched)
	}
}

// TestPipelineGivesProvisionalAnswerWhenResearchFails is requirement 8: a
// research-provider failure degrades the answer and labels it, and is never
// reported as an error and never as an empty result.
func TestPipelineGivesProvisionalAnswerWhenResearchFails(t *testing.T) {
	for name, err := range map[string]error{
		"unavailable":  research.ErrUnavailable,
		"timeout":      research.ErrTimeout,
		"rate limited": research.ErrRateLimited,
	} {
		t.Run(name, func(t *testing.T) {
			p := &Pipeline{
				Suggester: &fakeSuggester{result: okReply()},
				Research:  &fakeResearch{err: err},
			}

			outcome := p.Run(context.Background(), Request{Intention: "remembering someone"})
			if outcome.Err != nil {
				t.Fatalf("a research failure must not fail the discovery, got %v", outcome.Err)
			}
			if len(outcome.Result.Candidates) == 0 {
				t.Fatal("the visitor got no candidates: a research outage must not cost them an answer")
			}
			if outcome.Status != StatusUnavailable {
				t.Errorf("status = %q, want %q: the answer must be labelled un-researched", outcome.Status, StatusUnavailable)
			}
			if !strings.Contains(outcome.Result.ResearchSummary, "Not researched") {
				t.Errorf("researchSummary = %q, must say the answer was not researched", outcome.Result.ResearchSummary)
			}
			if outcome.Result.ResearchStatus != ai.ResearchStatusUnavailable {
				t.Errorf("the model was told researchStatus=%q, want %q so it cannot claim a source",
					outcome.Result.ResearchStatus, ai.ResearchStatusUnavailable)
			}
		})
	}
}

// TestPipelineTellsPartialResearchApartFromFull is the difference between "we
// looked everywhere we could" and "we looked in some of the places".
func TestPipelineTellsPartialResearchApartFromFull(t *testing.T) {
	p := &Pipeline{
		Suggester: &fakeSuggester{result: okReply()},
		Research:  &fakeResearch{sources: []research.Source{aSource("x", "One")}, err: research.ErrPartial},
	}

	outcome := p.Run(context.Background(), Request{Intention: "curiosity"})
	if outcome.Status != StatusPartial {
		t.Errorf("status = %q, want %q", outcome.Status, StatusPartial)
	}
	if len(outcome.Sources) == 0 {
		t.Error("the findings from the backend that answered must survive")
	}
}

// TestPipelineReportsAProviderFailureAsAnError is the other half of
// requirement 8: an inference failure is an error, never an empty answer that
// reads as "Everroot has nothing for you".
func TestPipelineReportsAProviderFailureAsAnError(t *testing.T) {
	p := &Pipeline{
		Suggester: &fakeSuggester{err: ai.ErrRateLimited},
		Research:  &fakeResearch{sources: []research.Source{aSource("x", "One")}},
	}

	outcome := p.Run(context.Background(), Request{Intention: "hope"})
	if outcome.Err == nil {
		t.Fatal("a provider failure must surface as an error, not as a discovery with no candidates")
	}
	if len(outcome.Result.Candidates) != 0 {
		t.Error("no candidates may accompany a provider error")
	}
}

// TestPipelineRefusesToRunWithoutAModel pins that an unconfigured model is a
// named state rather than a silent empty answer.
func TestPipelineRefusesToRunWithoutAModel(t *testing.T) {
	p := &Pipeline{Research: &fakeResearch{}}
	outcome := p.Run(context.Background(), Request{Intention: "x"})
	if !errors.Is(outcome.Err, ai.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", outcome.Err)
	}
}

// TestPipelineDerivesResearchTermsFromTheVisitor proves the research phase is
// driven by what the visitor actually said, with no model call spent on
// deciding what to search for.
func TestPipelineDerivesResearchTermsFromTheVisitor(t *testing.T) {
	researcher := &fakeResearch{}
	p := &Pipeline{Suggester: &fakeSuggester{result: okReply()}, Research: researcher}

	p.Run(context.Background(), Request{
		Intention:   "a tree for the festival",
		Description: "something that flowers in the monsoon",
		Tradition:   "Tamil",
		Region:      "Tamil Nadu",
		Nakshatra:   "Ashwini",
	})

	got := researcher.got.String()
	for _, want := range []string{"Ashwini", "Tamil", "Tamil Nadu", "monsoon"} {
		if !strings.Contains(got, want) {
			t.Errorf("research query %q is missing the visitor's own term %q", got, want)
		}
	}
}

// TestPipelineSendsRetrievedSourcesWithServerAssignedIDs proves the model sees
// the findings and cites them by an ID the server controls.
func TestPipelineSendsRetrievedSourcesWithServerAssignedIDs(t *testing.T) {
	sug := &fakeSuggester{result: okReply()}
	p := &Pipeline{
		Suggester: sug,
		Research: &fakeResearch{sources: []research.Source{
			{Title: "First", URL: "https://example.org/1", Snippet: "one"},
			{Title: "Second", URL: "https://example.org/2", Snippet: "two"},
		}},
	}

	p.Run(context.Background(), Request{Intention: "x"})

	if len(sug.got.Sources) != 2 {
		t.Fatalf("model saw %d sources, want 2", len(sug.got.Sources))
	}
	if sug.got.Sources[0].ID != "s1" || sug.got.Sources[1].ID != "s2" {
		t.Errorf("IDs = %q,%q; want server-assigned s1,s2 in a fixed order",
			sug.got.Sources[0].ID, sug.got.Sources[1].ID)
	}
	// The snippet travels, so the model can weigh relevance.
	if sug.got.Sources[0].Snippet != "one" {
		t.Errorf("snippet = %q, want the retrieved extract", sug.got.Sources[0].Snippet)
	}
}

// TestPipelineTellsTheModelWhenNothingWasRetrieved proves the model is
// explicitly told research was unavailable, which is the only thing that
// stops it inventing a citation to fill the gap.
func TestPipelineTellsTheModelWhenNothingWasRetrieved(t *testing.T) {
	sug := &fakeSuggester{result: okReply()}
	p := &Pipeline{Suggester: sug, Research: &fakeResearch{err: research.ErrUnavailable}}

	p.Run(context.Background(), Request{Intention: "x"})

	if sug.got.ResearchStatus != ai.ResearchStatusUnavailable {
		t.Errorf("researchStatus = %q, want %q", sug.got.ResearchStatus, ai.ResearchStatusUnavailable)
	}
	if sug.got.Sources != nil {
		t.Errorf("sources = %+v, want none", sug.got.Sources)
	}
}

// TestPipelineVerifiesBotanicalIdentity is requirement 7 of the brief: a
// scientific name is checked against a taxonomic record rather than trusted.
func TestPipelineVerifiesBotanicalIdentity(t *testing.T) {
	verifier := &fakeVerifier{identity: research.Identity{
		Matched:        true,
		ScientificName: "Saraca asoca (Roxb.) Dekker",
		Family:         "Fabaceae",
		Source:         "https://www.gbif.org/species/5023541",
	}}
	p := &Pipeline{
		Suggester: &fakeSuggester{result: okReply()},
		Research:  &fakeResearch{sources: []research.Source{aSource("x", "One")}},
		Verifier:  verifier,
	}

	outcome := p.Run(context.Background(), Request{Intention: "x"})

	if len(verifier.asked) != 1 || verifier.asked[0] != "Saraca asoca" {
		t.Fatalf("verifier asked about %v, want the candidate's binomial", verifier.asked)
	}
	identity := outcome.Result.Candidates[0].Identity
	if identity.Status != ai.IdentityConfirmed {
		t.Errorf("identity status = %q, want %q", identity.Status, ai.IdentityConfirmed)
	}
	if identity.Family != "Fabaceae" {
		t.Errorf("family = %q", identity.Family)
	}
	if identity.Source == "" {
		t.Error("a confirmed identity must carry the taxonomic record's own URL")
	}
}

// TestPipelineNeverMarksAnUnverifiedNameAsConfirmed is the failure mode that
// matters most: an unconfirmed binomial must never be able to read as checked.
func TestPipelineNeverMarksAnUnverifiedNameAsConfirmed(t *testing.T) {
	cases := map[string]struct {
		verifier Verifier
		want     string
	}{
		"no verifier configured": {nil, ai.IdentityNotAttempted},
		"lookup failed":          {&fakeVerifier{err: research.ErrTimeout}, ai.IdentityUnverified},
		"name not found":         {&fakeVerifier{identity: research.Identity{}}, ai.IdentityNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := &Pipeline{
				Suggester: &fakeSuggester{result: okReply()},
				Research:  &fakeResearch{sources: []research.Source{aSource("x", "One")}},
				Verifier:  tc.verifier,
			}
			outcome := p.Run(context.Background(), Request{Intention: "x"})
			got := outcome.Result.Candidates[0].Identity.Status
			if got != tc.want {
				t.Errorf("identity status = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPipelineSkipsANamelessCandidate: a candidate with no binomial has nothing
// to verify, and must not turn into a lookup for an empty string.
func TestPipelineSkipsANamelessCandidate(t *testing.T) {
	verifier := &fakeVerifier{}
	p := &Pipeline{
		Suggester: &fakeSuggester{result: ai.DiscoverResult{Candidates: []ai.Candidate{{Name: "No binomial"}}}},
		Research:  &fakeResearch{sources: []research.Source{aSource("x", "One")}},
		Verifier:  verifier,
	}
	outcome := p.Run(context.Background(), Request{Intention: "x"})
	if len(verifier.asked) != 0 {
		t.Errorf("verifier was asked about %v; a nameless candidate must not trigger a lookup", verifier.asked)
	}
	if got := outcome.Result.Candidates[0].Identity.Status; got != ai.IdentityNotAttempted {
		t.Errorf("identity status = %q, want %q", got, ai.IdentityNotAttempted)
	}
}

func TestSourcesForResolvesCitationsInOrder(t *testing.T) {
	sources := []research.Source{aSource("s1", "One"), aSource("s2", "Two"), aSource("s3", "Three")}
	got := SourcesFor(sources, []string{"s3", "s1"})
	if len(got) != 2 || got[0].ID != "s3" || got[1].ID != "s1" {
		t.Fatalf("SourcesFor = %+v, want the cited sources in citation order", got)
	}
	// An unresolvable citation is skipped rather than rendered as a hole.
	if got := SourcesFor(sources, []string{"s1", "ghost"}); len(got) != 1 {
		t.Errorf("an unknown ID must be dropped, got %+v", got)
	}
}

func TestDistinctSourcesFlattensAndDeduplicates(t *testing.T) {
	sources := []research.Source{aSource("s1", "One"), aSource("s2", "Two")}
	candidates := []ai.Candidate{
		{SourceIDs: []string{"s2", "s1"}},
		{SourceIDs: []string{"s1"}},
	}
	got := DistinctSources(sources, candidates)
	if len(got) != 2 {
		t.Fatalf("got %d sources, want 2 unique", len(got))
	}
	if got[0].ID != "s1" || got[1].ID != "s2" {
		t.Errorf("order = %q,%q; want a stable sorted order", got[0].ID, got[1].ID)
	}
}

func TestResearchRespectsItsOwnTimeout(t *testing.T) {
	slow := slowResearch{d: 50 * time.Millisecond}
	p := &Pipeline{
		Suggester:       &fakeSuggester{result: okReply()},
		Research:        &slow,
		ResearchTimeout: 5 * time.Millisecond,
	}

	outcome := p.Run(context.Background(), Request{Intention: "x"})
	// The slow backend is cut off; the discovery still completes and is
	// labelled un-researched rather than failing.
	if outcome.Err != nil {
		t.Fatalf("Run: %v", outcome.Err)
	}
	if outcome.Status != StatusUnavailable {
		t.Errorf("status = %q, want %q", outcome.Status, StatusUnavailable)
	}
}

type slowResearch struct {
	d time.Duration
}

func (s *slowResearch) Name() string { return "slow" }
func (s *slowResearch) Search(ctx context.Context, _ research.Query, _ int) ([]research.Source, error) {
	select {
	case <-time.After(s.d):
		return []research.Source{aSource("late", "Late")}, nil
	case <-ctx.Done():
		return nil, research.ErrTimeout
	}
}
