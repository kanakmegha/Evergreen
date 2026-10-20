package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"everroot/internal/ai"
)

// recommender is the slice of the AI client this package needs. A nil
// recommender means the suggestion layer is switched off, which is reported as
// unconfigured — never as an answer that happens to contain nothing.
type recommender interface {
	SuggestTrees(ctx context.Context, in ai.RecommendInput) (ai.SuggestResult, error)
}

// Bounds on an incoming recommendation request. They are deliberately generous
// beside what the prompt finally sends: a request that trips them is malformed,
// while the prompt layer trims anything that is merely long.
const (
	recommendTimeout      = 30 * time.Second
	defaultRecommendLimit = ai.DefaultSuggestions
	// maxRecommendListItems and maxRecommendItemChars bound the free-form lists.
	// The prompt's own limits are smaller and live in the AI package, which is
	// the layer that actually assembles the request sent to the provider.
	maxRecommendListItems = 20
	maxRecommendItemChars = 500
)

type recommendationsRequest struct {
	Intention string `json:"intention"`
	// Answer is the visitor's own words. It is required by neither the shape of
	// the form nor this handler; it travels only because a suggestion drawn
	// without it would be a guess.
	Answer string   `json:"answer"`
	Choice string   `json:"choice"`
	Picks  []string `json:"picks"`
	// Tradition, Region and Nakshatra are context for the prompt and the input
	// to the reviewed-association check. They are never required.
	Tradition   string   `json:"tradition"`
	Region      string   `json:"region"`
	Nakshatra   string   `json:"nakshatra"`
	Constraints []string `json:"constraints"`
	Limit       int      `json:"limit"`
}

type recommendationsResponse struct {
	// Status is "ok" when suggestions were made and "no_suggestions" when the
	// model found nothing it would stand behind. Both are successful answers;
	// a failure never reaches this struct.
	Status        string          `json:"status"`
	Suggestions   []ai.Suggestion `json:"suggestions"`
	Evidence      evidenceStatus  `json:"evidence"`
	PromptVersion string          `json:"promptVersion"`
	Model         string          `json:"model"`
	Disclaimer    string          `json:"disclaimer"`
	// FollowUpQuestion is present only when one missing fact would materially
	// change the suggestions. It is omitted rather than sent empty, so a
	// consumer never has to distinguish "no question" from "no answer".
	FollowUpQuestion string `json:"followUpQuestion,omitempty"`
}

// Status values this endpoint reports with a 200.
const (
	recommendStatusOK            = "ok"
	recommendStatusNoSuggestions = "no_suggestions"
)

// recommend handles POST /api/v1/recommendations.
//
// It answers a different question from POST /api/v1/discoveries, and keeping
// them apart is the point:
//
//	POST /api/v1/discoveries asks the publication gate whether a reviewed
//	cultural association exists for a selection. It can only ever return what a
//	reviewer approved, and its "no" is a fact about Everroot's records.
//
//	POST /api/v1/recommendations asks the configured model what else might fit
//	this visitor's answers, whatever tradition they picked. It is machine
//	written, it carries no source, and it never claims a cultural association —
//	which is exactly why it may exist for an intention that has no reviewed
//	mapping at all.
//
// It needs neither the database nor the botanical catalogue, so a suggestion
// still arrives when the catalogue is empty or unreachable. When the model
// fails it reports the failure; it never answers with an empty list that would
// read as "Everroot has nothing for you".
func (a *API) recommend(w http.ResponseWriter, r *http.Request) {
	var req recommendationsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid JSON request")
		return
	}

	req.Intention = strings.TrimSpace(req.Intention)
	label, supported := intentionLabels[req.Intention]
	if !supported {
		writeError(w, 400, "unsupported intention")
		return
	}
	req.Answer = strings.TrimSpace(req.Answer)
	req.Choice = strings.TrimSpace(req.Choice)
	req.Tradition = strings.TrimSpace(req.Tradition)
	req.Region = strings.TrimSpace(req.Region)
	req.Nakshatra = strings.TrimSpace(req.Nakshatra)

	if len(req.Answer) > ai.MaxPromptAnswerChars {
		writeError(w, 400, "answer is too long (maximum 700 characters)")
		return
	}
	if len(req.Choice) > 120 || len(req.Tradition) > 100 || len(req.Region) > 100 || len(req.Nakshatra) > 100 {
		writeError(w, 400, "invalid selection")
		return
	}
	if len(req.Picks) > maxRecommendListItems || len(req.Constraints) > maxRecommendListItems ||
		anyTooLong(req.Picks, maxRecommendItemChars) || anyTooLong(req.Constraints, maxRecommendItemChars) {
		writeError(w, 400, "too many choices, or a choice that is too long")
		return
	}
	limit := defaultRecommendLimit
	if req.Limit != 0 {
		if req.Limit < 1 || req.Limit > ai.MaxSuggestions {
			writeError(w, 400, "limit must be between 1 and 5")
			return
		}
		limit = req.Limit
	}

	if a.recommender == nil {
		a.log.Warn("AI tree suggestions requested but AI client is not configured",
			"request_id", r.Header.Get("X-Request-ID"))
		status, message := aiFailureResponse(ai.Diagnosis{Category: ai.CategoryNotConfigured}, "recommendation")
		writeError(w, status, message)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), recommendTimeout)
	defer cancel()

	// The publication gate answers the cultural half of the reply. It is the
	// same predicate the explanation and discovery endpoints use, so a
	// suggestion can never be shown beside a cultural claim Track A would deny.
	//
	// It reports a verdict and refuses nothing. A tradition Everroot does not
	// carry used to make this endpoint answer 400, which turned "nobody has
	// reviewed this combination" into a refusal to help — the visitor picked a
	// real tradition, and the honest reply is a suggestion beside an honest
	// "no reviewed record", not an error.
	association := a.associationStatus(ctx, req.Tradition, req.Region, req.Nakshatra)

	result, err := a.recommender.SuggestTrees(ctx, ai.RecommendInput{
		IntentionID:       req.Intention,
		IntentionLabel:    label,
		Choice:            req.Choice,
		Answer:            req.Answer,
		Picks:             req.Picks,
		Tradition:         req.Tradition,
		Region:            req.Region,
		Nakshatra:         req.Nakshatra,
		Constraints:       req.Constraints,
		AssociationStatus: association,
		Count:             limit,
	})
	if err != nil {
		diag := ai.Diagnose(err)
		// Only the classification is logged: the request carries the visitor's
		// own words and the reply carries model output, neither of which has
		// any business in a log line. diag.Detail is built to be credential-free
		// and is the one piece of text safe to record.
		a.log.Error("AI tree suggestions failed",
			"request_id", r.Header.Get("X-Request-ID"),
			"category", string(diag.Category),
			"status", diag.Status,
			"provider_code", diag.ProviderCode,
			"retryable", diag.Retryable,
			"detail", diag.Detail,
		)
		if diag.RetryAfter > 0 {
			w.Header().Set("Retry-After", retryAfterSeconds(diag.RetryAfter))
		}
		status, message := aiFailureResponse(diag, "recommendation")
		writeError(w, status, message)
		return
	}

	suggestions := result.Suggestions
	if suggestions == nil {
		suggestions = []ai.Suggestion{}
	}
	status := recommendStatusOK
	if len(suggestions) == 0 {
		// A model that found nothing says so; it is never dressed up as a
		// successful answer with an empty array and no explanation.
		status = recommendStatusNoSuggestions
	}

	writeJSON(w, 200, recommendationsResponse{
		Status:           status,
		Suggestions:      suggestions,
		Evidence:         a.recommendationEvidence(association),
		PromptVersion:    result.PromptVersion,
		Model:            result.Model,
		Disclaimer:       result.Disclaimer,
		FollowUpQuestion: result.FollowUpQuestion,
	})
}

// recommendationEvidence is the evidence panel for a machine-written
// suggestion. Only the cultural half comes from Everroot's own gate; every
// other dimension is reported as absent from the reviewed records, because a
// model's prose is not a source and does not become one by being confident.
func (a *API) recommendationEvidence(association string) evidenceStatus {
	evidence := evidenceStatus{
		CulturalAssociation: association,
		BotanicalIdentity:   "unavailable",
		Habitat:             "no_reviewed_record",
		EcologicalRole:      "no_reviewed_record",
		DocumentedStories:   "no_reviewed_record",
		PersonalMeaning:     "user_owned",
	}
	if evidence.CulturalAssociation == "" {
		evidence.CulturalAssociation = assocNotQueryable
	}
	return evidence
}

func anyTooLong(items []string, max int) bool {
	for _, item := range items {
		if len(item) > max {
			return true
		}
	}
	return false
}
