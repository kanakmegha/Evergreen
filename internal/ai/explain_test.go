package ai

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// The prompt is the only thing standing between a language model and a
// fabricated cultural claim, so its guardrails are asserted directly.
func TestExplainPromptForbidsFabrication(t *testing.T) {
	prompt := strings.ToLower(explainSystemPrompt)
	for _, required := range []string{
		`only facts present in "species"`,
		"never state or imply that a tree is associated with a nakshatra",
		"never invent or name a source",
		"never say a tree has been planted",
		"never claim a connection between the visitor's personality",
		"say so in one sentence and stop",
		"never suggest that everroot provides, offers, recommends",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("system prompt lost a guardrail: %q", required)
		}
	}
}

// A missing client must fail loudly rather than return invented text.
func TestExplainWithoutClientIsExplicit(t *testing.T) {
	var client *Client
	if _, err := client.Explain(context.Background(), ExplainInput{Task: TaskExploreSelection}); err == nil {
		t.Fatal("nil client must return an error, not fabricated text")
	}
	var nilChat *Client
	if _, err := nilChat.ChatCompletion(context.Background(), nil); err == nil {
		t.Fatal("nil client must return an error")
	}
}

func TestCleanExplanationTrimsAndCaps(t *testing.T) {
	if got := cleanExplanation("  hello \n"); got != "hello" {
		t.Fatalf("trim failed: %q", got)
	}
	if got := cleanExplanation("   "); got != "" {
		t.Fatalf("blank text must become empty: %q", got)
	}
	long := strings.Repeat("a", maxExplainChars+500)
	if got := cleanExplanation(long); len(got) != maxExplainChars {
		t.Fatalf("length cap failed: %d", len(got))
	}
}

// An answer that stops mid-sentence reads as a fault rather than as an honest
// limit, so a trailing fragment is dropped.
func TestCleanExplanationDropsTrailingFragment(t *testing.T) {
	cut := "Everroot holds two authority records for these species. The first is"
	got := cleanExplanation(cut)
	if got != "Everroot holds two authority records for these species." {
		t.Fatalf("trailing fragment should be dropped: %q", got)
	}

	// With no complete sentence to fall back on, an over-long answer is marked
	// as cut rather than passed off as finished.
	long := strings.Repeat("word ", 200)
	marked := cleanExplanation(long)
	if len(marked) > maxExplainChars {
		t.Fatalf("cap exceeded: %d", len(marked))
	}
	if !strings.HasSuffix(marked, "…") {
		t.Fatalf("a hard cut must be marked: %q", marked)
	}
}

// If the text has to be cut, the cut lands on a sentence the model finished.
func TestCleanExplanationPrefersSentenceBoundary(t *testing.T) {
	first := "Everroot holds an authority record for this species."
	// Comfortably more than the cap, so the cut has to happen somewhere.
	text := first + " " + strings.Repeat("and it says nothing about local practice. ", 40)

	got := cleanExplanation(text)
	if len(got) > maxExplainChars {
		t.Fatalf("cap exceeded: %d", len(got))
	}
	if !strings.HasPrefix(text, got) {
		t.Fatalf("the answer must be a prefix of the model output, not reworded: %q", got)
	}
	if !strings.HasSuffix(got, ".") {
		t.Fatalf("the answer should end on a finished sentence: %q", got)
	}
	if len(text) > len(got) && !strings.HasPrefix(text[len(got):], " ") {
		t.Fatalf("the cut did not land on a sentence boundary: %q", got)
	}
}

// TestLiveExplainWithHuggingFace verifies that when HF_TOKEN is configured,
// the client can connect to Hugging Face Inference Providers and obtain a
// grounded explanation. If HF_TOKEN is missing, the test is skipped.
func TestLiveExplainWithHuggingFace(t *testing.T) {
	if os.Getenv("HF_TOKEN") == "" {
		t.Skip("HF_TOKEN is not configured; skipping live Hugging Face test")
	}
	client, err := New(nil)
	if err != nil {
		t.Fatalf("failed to initialize client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	result, err := client.Explain(ctx, ExplainInput{
		Task: TaskExploreSelection,
		Selection: SelectionFacts{
			Intention: "personal reflection",
			Tradition: "vedic",
			Nakshatra: "ashwini",
		},
		Species: []SpeciesFacts{
			{
				AcceptedName: "Ficus religiosa",
				Family:       "Moraceae",
				CommonNames:  []string{"Sacred Fig", "Peepal"},
				Authority:    "GBIF Backbone Taxonomy (doi:10.15468/39omei)",
			},
		},
		NotOnRecord: []string{
			"a reviewed cultural association for this exact selection",
		},
	})
	if err != nil {
		t.Fatalf("live Explain failed: %v", err)
	}
	if strings.TrimSpace(result.Text) == "" {
		t.Fatal("expected non-empty explanation text")
	}
	if result.Model == "" {
		t.Fatal("expected non-empty model identifier")
	}
}
