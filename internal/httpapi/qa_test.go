package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"everroot/internal/ai"
)

type stubQA struct {
	result       ai.AnswerResult
	err          error
	called       bool
	lastQuestion ai.QuestionInput
}

func (s *stubQA) AnswerQuestion(_ context.Context, in ai.QuestionInput) (ai.AnswerResult, error) {
	s.called = true
	s.lastQuestion = in
	return s.result, s.err
}

// 1. Test: An AI answer can be generated when PostgreSQL is unavailable (db = nil).
func TestAIAnswerGeneratedWhenPostgreSQLUnavailable(t *testing.T) {
	stub := &stubQA{
		result: ai.AnswerResult{
			Question:      "What family does the Jamun tree belong to?",
			Answer:        "The Jamun tree (Syzygium cumini) is in the family Myrtaceae.",
			PromptVersion: ai.QuestionPromptVersion,
			Model:         "openai/gpt-oss-120b:cheapest",
			Disclaimer:    "Grounded test disclaimer",
		},
	}

	// Initialize API with nil db (PostgreSQL is completely unavailable)
	api := &API{
		db:  nil,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		mux: http.NewServeMux(),
		qa:  stub,
	}
	api.routes()

	payload := map[string]any{
		"question":  "What family does the Jamun tree belong to?",
		"sessionId": "test-session-123",
		"intention": "culture",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/questions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp questionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
	if !strings.Contains(resp.Answer, "Myrtaceae") {
		t.Errorf("expected answer to contain 'Myrtaceae', got: %s", resp.Answer)
	}
	if resp.PromptVersion != ai.QuestionPromptVersion {
		t.Errorf("expected prompt version %s, got %s", ai.QuestionPromptVersion, resp.PromptVersion)
	}
	if resp.InteractionID == "" {
		t.Errorf("expected interactionId to be generated even when DB is nil")
	}
	if !stub.called {
		t.Errorf("expected AI provider to be called")
	}
}

// 2. Test: A successful AI answer is still returned when persistence fails.
func TestSuccessfulAIAnswerReturnedWhenPersistenceFails(t *testing.T) {
	stub := &stubQA{
		result: ai.AnswerResult{
			Question:      "What is the significance of the Peepal tree?",
			Answer:        "The Peepal tree (Ficus religiosa) holds deep sacred significance across South Asia.",
			PromptVersion: ai.QuestionPromptVersion,
			Model:         "openai/gpt-oss-120b:cheapest",
			Disclaimer:    "Test disclaimer",
		},
	}

	// db is nil (or broken connection); persistence to PostgreSQL fails/is skipped
	api := &API{
		db:  nil, // simulates DB failure during persistence
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		mux: http.NewServeMux(),
		qa:  stub,
	}
	api.routes()

	payload := map[string]any{
		"question": "What is the significance of the Peepal tree?",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/questions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.mux.ServeHTTP(w, req)

	// Even if persistence fails or DB is unavailable, HTTP 200 and the answer MUST be returned
	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp questionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Answer, "Peepal tree") {
		t.Errorf("expected answer to contain 'Peepal tree', got: %s", resp.Answer)
	}
}

// 3. Test: The question-answer endpoint calls the AI provider rather than fetching an answer from a database catalogue.
func TestQuestionEndpointCallsAIProviderRatherThanDBCatalogue(t *testing.T) {
	aiCalled := false
	stub := &stubQA{
		result: ai.AnswerResult{
			Question:      "Tell me about a tree for healing",
			Answer:        "Neem (Azadirachta indica) is widely respected for its traditional healing properties.",
			PromptVersion: ai.QuestionPromptVersion,
			Model:         "openai/gpt-oss-120b:cheapest",
		},
	}

	// Ensure catalogue is empty/unconfigured
	api := &API{
		db:        nil,
		catalogue: stubTreeSource{facts: nil}, // empty catalogue
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		mux:       http.NewServeMux(),
		qa:        stub,
	}
	api.routes()

	payload := map[string]any{
		"question": "Tell me about a tree for healing",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/questions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}
	aiCalled = stub.called
	if !aiCalled {
		t.Fatalf("expected request to be answered by AI provider call, but AI was not called")
	}

	var resp questionResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp.Answer, "Neem") {
		t.Errorf("expected AI answer about Neem, got %s", resp.Answer)
	}
}

// 4. Test: Feedback and interaction persistence work independently of answer generation.
func TestFeedbackAndInteractionPersistenceWorkIndependently(t *testing.T) {
	api := &API{
		db:  nil, // DB offline
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		mux: http.NewServeMux(),
	}
	api.routes()

	// Submitting feedback when DB is offline returns graceful status without failing the client
	payload := map[string]any{
		"interactionId": "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
		"feedbackType":  "answer",
		"rating":        1,
		"comment":       "Very clear explanation.",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

// Test AI Provider Failure returns clean error and does NOT fall back to DB.
func TestAIProviderFailureReturnsErrorWithoutDBFallback(t *testing.T) {
	stub := &stubQA{
		err: errors.New("hugging face timeout"),
	}

	api := &API{
		db:  nil,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		mux: http.NewServeMux(),
		qa:  stub,
	}
	api.routes()

	payload := map[string]any{
		"question": "Tell me about Ashwini",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/questions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.mux.ServeHTTP(w, req)

	// Must return a 503 error, not silently fabricate or fall back to DB catalogue
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP 503 on AI failure, got %d", w.Code)
	}
}

// An unconfigured deployment must say so plainly. Before the interface was
// guarded, a nil *ai.Client was stored in the qa field, so `a.qa == nil` was
// false and the handler reported a misleading "temporarily unavailable".
func TestUnconfiguredAIReportsNotConfigured(t *testing.T) {
	t.Setenv("HF_TOKEN", "")
	t.Setenv("HF_MODEL_ID", "")

	h, err := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"question": "What family does Sal belong to?"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/questions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP 503 when AI is unconfigured, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not configured") {
		t.Fatalf("expected a 'not configured' message, got %s", w.Body.String())
	}
}
