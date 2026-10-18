package httpapi

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"time"

	"everroot/internal/ai"
)

// newUUID generates a cryptographically random RFC 4122 v4 UUID.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type questionRequest struct {
	Question  string            `json:"question"`
	SessionID string            `json:"sessionId"`
	Intention string            `json:"intention"`
	Context   map[string]string `json:"context"`
}

type questionResponse struct {
	Status        string `json:"status"`
	InteractionID string `json:"interactionId"`
	Question      string `json:"question"`
	Answer        string `json:"answer"`
	PromptVersion string `json:"promptVersion"`
	Model         string `json:"model"`
	Disclaimer    string `json:"disclaimer"`
}

// askQuestion handles POST /api/v1/questions and POST /api/v1/ask.
//
// The core product requirement is satisfied here:
// 1. The user submits a question through the website.
// 2. The Go backend validates input and builds the prompt.
// 3. The backend calls the configured Hugging Face model API.
// 4. The backend returns the model-generated answer to the frontend.
// 5. Separately, the interaction is persisted to PostgreSQL if available.
//
// PostgreSQL is not required for answer generation: a database outage never
// prevents an otherwise successful AI API call from answering.
func (a *API) askQuestion(w http.ResponseWriter, r *http.Request) {
	var req questionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid JSON request")
		return
	}

	q := strings.TrimSpace(req.Question)
	if q == "" {
		writeError(w, 400, "question cannot be empty")
		return
	}
	if len(q) > 1000 {
		writeError(w, 400, "question is too long (maximum 1000 characters)")
		return
	}
	if len(req.Intention) > 100 {
		writeError(w, 400, "intention is too long")
		return
	}

	if a.qa == nil {
		a.log.Warn("AI answer generation requested but AI client is not configured",
			"request_id", r.Header.Get("X-Request-ID"))
		writeError(w, 503, "The AI question service is not configured. Please ensure HF_TOKEN is set.")
		return
	}

	// 2 & 3: Validate input, build prompt, and call external Hugging Face AI API
	input := ai.QuestionInput{
		Question:  q,
		SessionID: strings.TrimSpace(req.SessionID),
		Intention: strings.TrimSpace(req.Intention),
		Context:   req.Context,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := a.qa.AnswerQuestion(ctx, input)
	if err != nil {
		a.log.Error("AI provider error during question answering",
			"request_id", r.Header.Get("X-Request-ID"),
			"error", err,
		)
		// Do not silently fall back to database queries; return explicit provider error
		writeError(w, 503, "The AI service is temporarily unavailable. Please try again.")
		return
	}

	// 5: Separately and non-blockingly attempt persistence.
	// A failure to save to PostgreSQL never blocks or fails returning the answer.
	interactionID := a.saveInteraction(r.Context(), input.SessionID, result.Question, result.Answer, result.PromptVersion, result.Model)

	// 4: Return model-generated answer to frontend
	writeJSON(w, 200, questionResponse{
		Status:        "ok",
		InteractionID: interactionID,
		Question:      result.Question,
		Answer:        result.Answer,
		PromptVersion: result.PromptVersion,
		Model:         result.Model,
		Disclaimer:    result.Disclaimer,
	})
}

// saveInteraction saves the question, answer, and prompt version to PostgreSQL if available.
// If the database is nil or an error occurs, it safely logs the issue and returns a fallback UUID.
func (a *API) saveInteraction(ctx context.Context, sessionID, question, answer, promptVersion, model string) string {
	fallbackID := newUUID()
	if a.db == nil {
		return fallbackID
	}

	dbCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var id string
	err := a.db.QueryRow(dbCtx, `
		INSERT INTO user_interactions (session_id, question, answer, prompt_version, model)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text
	`, nullIfEmpty(sessionID), question, answer, promptVersion, model).Scan(&id)

	if err != nil {
		// Log persistence failure safely without logging sensitive user content
		a.log.Warn("Failed to persist user interaction to database; continuing successfully", "error", err)
		return fallbackID
	}
	return id
}

type feedbackRequest struct {
	InteractionID string `json:"interactionId"`
	FeedbackType  string `json:"feedbackType"` // 'answer' or 'website'
	Rating        *int   `json:"rating"`       // e.g. 1 for helpful, -1 for unhelpful
	Comment       string `json:"comment"`
	Correction    string `json:"correction"`
}

// submitFeedback handles POST /api/v1/feedback.
func (a *API) submitFeedback(w http.ResponseWriter, r *http.Request) {
	var req feedbackRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid JSON request")
		return
	}

	feedbackType := strings.TrimSpace(req.FeedbackType)
	if feedbackType == "" {
		feedbackType = "answer"
	}
	if len(feedbackType) > 50 {
		writeError(w, 400, "invalid feedback type")
		return
	}
	if len(req.Comment) > 2000 || len(req.Correction) > 2000 {
		writeError(w, 400, "comment or correction exceeds maximum length")
		return
	}

	if a.db == nil {
		// If database is not configured, acknowledge receipt gracefully
		writeJSON(w, 200, map[string]string{
			"status":  "ok",
			"message": "Feedback received (persistence currently offline)",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var interactionID any
	req.InteractionID = strings.TrimSpace(req.InteractionID)
	if req.InteractionID != "" && uuidPattern.MatchString(req.InteractionID) {
		interactionID = req.InteractionID
	}

	_, err := a.db.Exec(ctx, `
		INSERT INTO user_feedback (interaction_id, feedback_type, rating, comment, correction)
		VALUES ($1, $2, $3, $4, $5)
	`, interactionID, feedbackType, req.Rating, nullIfEmpty(req.Comment), nullIfEmpty(req.Correction))

	if err != nil {
		a.log.Warn("Failed to persist user feedback", "error", err)
		writeError(w, 503, "Failed to record feedback")
		return
	}

	writeJSON(w, 200, map[string]string{
		"status":  "ok",
		"message": "Thank you for your feedback",
	})
}

func nullIfEmpty(s string) any {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return trimmed
}
