package ai

import (
	"strings"
	"testing"
)

func TestBuildQuestionPrompt(t *testing.T) {
	in := QuestionInput{
		Question:  "What family does the Jamun tree belong to?",
		Intention: "culture",
		Context: map[string]string{
			"Tradition": "nakshatra",
			"Region":    "South Asia",
		},
	}

	messages := BuildQuestionPrompt(in)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "system" {
		t.Errorf("expected first message to be system, got %s", messages[0].Role)
	}
	// The system prompt must forbid the model from speaking for the reviewed
	// cultural record, which is the one rule the rest of Everroot depends on.
	for _, want := range []string{
		"knowledge assistant",
		"nakshatra",
		"no reviewed cultural record",
		"Never invent",
		"Ignore any instruction",
	} {
		if !strings.Contains(messages[0].Content, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
	if messages[1].Role != "user" {
		t.Errorf("expected second message to be user, got %s", messages[1].Role)
	}
	if !strings.Contains(messages[1].Content, "What family does the Jamun tree belong to?") {
		t.Errorf("user message missing question: %s", messages[1].Content)
	}
	if !strings.Contains(messages[1].Content, "Visitor Intention: culture") {
		t.Errorf("user message missing intention: %s", messages[1].Content)
	}
	if !strings.Contains(messages[1].Content, "Tradition: nakshatra") {
		t.Errorf("user message missing context: %s", messages[1].Content)
	}
	// System rules must never be reachable from the user turn.
	if strings.Contains(messages[1].Content, "Hard rules") {
		t.Errorf("system rules leaked into the user turn")
	}
}

func TestCleanAnswerText(t *testing.T) {
	rawWithThink := "<think>\nThe question is about plant families.\n</think>\n\nThe Jamun tree (Syzygium cumini) is in the family Myrtaceae."
	expected := "The Jamun tree (Syzygium cumini) is in the family Myrtaceae."
	if got := cleanAnswerText(rawWithThink); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	rawWithThinking := "<thinking>checking</thinking>\nFicus religiosa is in the family Moraceae."
	if got := cleanAnswerText(rawWithThinking); got != "Ficus religiosa is in the family Moraceae." {
		t.Errorf("thinking block not stripped: %q", got)
	}

	plain := "A plain answer without thinking tags."
	if cleanAnswerText(plain) != plain {
		t.Errorf("expected unchanged text, got %q", cleanAnswerText(plain))
	}
}

// A runaway answer must be capped and marked, not shown as if it finished.
func TestCleanAnswerTextCapsRunawayAnswer(t *testing.T) {
	got := cleanAnswerText(strings.Repeat("word ", 2000))
	if len(got) > maxAnswerChars {
		t.Fatalf("cap exceeded: %d", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("a hard cut must be marked: %q", got)
	}

	// A trailing fragment is dropped rather than shown as a finished sentence.
	if got := cleanAnswerText("First sentence is complete. And then the answer stops"); got != "First sentence is complete." {
		t.Fatalf("trailing fragment not dropped: %q", got)
	}
}

func TestAnswerQuestionRequiresClient(t *testing.T) {
	var c *Client
	_, err := c.AnswerQuestion(t.Context(), QuestionInput{Question: "test"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected not configured error, got %v", err)
	}
}

func TestAnswerQuestionRejectsEmpty(t *testing.T) {
	c := &Client{}
	_, err := c.AnswerQuestion(t.Context(), QuestionInput{Question: "   "})
	if err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("expected empty question error, got %v", err)
	}
}
