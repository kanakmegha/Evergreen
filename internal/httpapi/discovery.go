package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"everroot/internal/ai"
	"everroot/internal/discovery"
	"everroot/internal/research"
)

// discoverTimeout bounds one AI-first discovery. It is larger than the
// suggestion endpoint's because a discovery also spends time on research and
// on a taxonomic lookup, and the three run inside this one bound rather than
// each getting their own.
const discoverTimeout = 75 * time.Second

// minIntentionChars and maxIntentionChars bound a free-text intention.
//
// The previous handler rejected any intention that was not one of five
// interface labels, which meant "a tree that means something to the Ahom" was
// refused with a 400 before any model saw it. A closed vocabulary is exactly
// the shape the product must not have: the paths are examples, not the limits
// of what Everroot will try to answer. The bounds below are about abuse, not
// about which questions are permitted.
const (
	minIntentionChars   = 3
	maxIntentionChars   = 300
	maxDescriptionChars = 2000
)

// aiDiscoveryRequest is one visitor's answers to the AI-first discovery flow.
type aiDiscoveryRequest struct {
	// Intention is either one of the interface's intention keys or the
	// visitor's own words. Anything that is not a known key is passed through
	// as free text, so an unfamiliar question reaches the model instead of a
	// 400.
	Intention string `json:"intention"`
	// Description is the visitor's own account of what they want.
	Description string `json:"description"`
	// Tradition, Region and Nakshatra are context the visitor chose to give.
	// None is required and none licenses a claim by itself.
	Tradition string `json:"tradition"`
	Region    string `json:"region"`
	Nakshatra string `json:"nakshatra"`
	// Constraints are practical requirements: space, light, climate, care.
	Constraints []string `json:"constraints"`
	Limit       int      `json:"limit"`
}

// aiDiscoveryResponse is the AI-first discovery reply.
type aiDiscoveryResponse struct {
	// Status is "ok" when candidates were produced and "no_candidates" when
	// the model found nothing it would stand behind. Both are successful
	// answers; a failure never reaches this struct.
	Status     string         `json:"status"`
	Candidates []ai.Candidate `json:"candidates"`
	// Sources are the retrieved findings, with the IDs the candidates cite.
	// A visitor can follow every citation to a real address.
	Sources []research.Source `json:"sources"`
	// ResearchStatus is "researched", "partial" or "unavailable". It is the
	// pipeline's own record of what happened, never the model's claim.
	ResearchStatus  string `json:"researchStatus"`
	ResearchSummary string `json:"researchSummary"`
	// UnverifiedAreas are the dimensions the sources did not settle.
	UnverifiedAreas []string `json:"unverifiedAreas,omitempty"`
	// FollowUpQuestion appears only when one missing fact would materially
	// change the answer.
	FollowUpQuestion string `json:"followUpQuestion,omitempty"`
	// ConfidenceDefinitions publishes what each confidence level means, so the
	// interface never has to imply a measured score.
	ConfidenceDefinitions map[string]string `json:"confidenceDefinitions"`
	PromptVersion         string            `json:"promptVersion"`
	Model                 string            `json:"model"`
	Disclaimer            string            `json:"disclaimer"`
	// RunID is present only when the discovery was saved. It names a row the
	// caller owns and can read back with their session token.
	RunID string `json:"runId,omitempty"`
	// Saved records whether the answer was kept. When it is false and a
	// session token was presented, the interface is expected to say the
	// discovery was not saved rather than implying that it was.
	Saved bool `json:"saved"`
}

// Status values this endpoint reports with a 200.
const (
	aiDiscoveryStatusOK           = "ok"
	aiDiscoveryStatusNoCandidates = "no_candidates"
)

// discover handles POST /api/v1/discovery, the AI-first discovery pipeline.
//
// It answers a different question from POST /api/v1/discoveries, and the two
// are kept apart on purpose:
//
//	POST /api/v1/discoveries asks the publication gate whether a reviewed
//	cultural association exists for a selection. It can only return what a
//	reviewer approved.
//
//	POST /api/v1/discovery asks the model to research and recommend, and
//	attributes every claim to the sources retrieved for that request. It is
//	machine-written and carries sources rather than a review.
//
// It reads no catalogue table, so a visitor gets an answer with the database
// empty or unreachable. Research failing is a labelled downgrade, not an
// error; a model failing is an error, never an empty list.
func (a *API) discoverAI(w http.ResponseWriter, r *http.Request) {
	var req aiDiscoveryRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid JSON request")
		return
	}

	intention, ok := resolveIntention(req.Intention)
	if !ok {
		writeError(w, 400, fmt.Sprintf("intention is required (between %d and %d characters)", minIntentionChars, maxIntentionChars))
		return
	}
	req.Description = strings.TrimSpace(req.Description)
	req.Tradition = strings.TrimSpace(req.Tradition)
	req.Region = strings.TrimSpace(req.Region)
	req.Nakshatra = strings.TrimSpace(req.Nakshatra)

	if len(req.Description) > maxDescriptionChars {
		writeError(w, 400, "description is too long")
		return
	}
	if len(req.Tradition) > 100 || len(req.Region) > 100 || len(req.Nakshatra) > 100 {
		writeError(w, 400, "invalid selection")
		return
	}
	if len(req.Constraints) > maxRecommendListItems || anyTooLong(req.Constraints, maxRecommendItemChars) {
		writeError(w, 400, "too many constraints, or a constraint that is too long")
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

	if a.pipeline == nil || a.pipeline.Suggester == nil {
		a.log.Warn("AI discovery requested but the AI client is not configured",
			"request_id", r.Header.Get("X-Request-ID"))
		status, message := aiFailureResponse(ai.Diagnosis{Category: ai.CategoryNotConfigured}, "discovery")
		writeError(w, status, message)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), discoverTimeout)
	defer cancel()

	outcome := a.pipeline.Run(ctx, discovery.Request{
		Intention:   intention,
		Description: req.Description,
		Tradition:   req.Tradition,
		Region:      req.Region,
		Nakshatra:   req.Nakshatra,
		Constraints: req.Constraints,
		Limit:       limit,
	})
	if outcome.Err != nil {
		diag := ai.Diagnose(outcome.Err)
		// Only the classification is logged. The request carries the visitor's
		// own words and the reply carries model output; neither belongs in a
		// log line, and diag.Detail is built to be credential-free.
		a.log.Error("AI discovery failed",
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
		status, message := aiFailureResponse(diag, "discovery")
		writeError(w, status, message)
		return
	}

	candidates := outcome.Result.Candidates
	if candidates == nil {
		candidates = []ai.Candidate{}
	}
	sources := outcome.Sources
	if sources == nil {
		sources = []research.Source{}
	}

	status := aiDiscoveryStatusOK
	if len(candidates) == 0 {
		// A model that found nothing says so. It is never dressed up as a
		// successful answer with an empty array and no explanation.
		status = aiDiscoveryStatusNoCandidates
	}

	// Persist when the caller presented a session token. A failure here never
	// costs the visitor the answer the pipeline just produced; it only means
	// the answer was not kept, which the response says plainly.
	runID, saved := a.saveOutcome(ctx, r, outcome, req)

	writeJSON(w, 200, aiDiscoveryResponse{
		Status:                status,
		Candidates:            candidates,
		Sources:               sources,
		ResearchStatus:        outcome.Status,
		ResearchSummary:       outcome.Result.ResearchSummary,
		UnverifiedAreas:       outcome.Result.Unverified,
		FollowUpQuestion:      outcome.Result.FollowUpQuestion,
		ConfidenceDefinitions: ai.ConfidenceDefinitions,
		PromptVersion:         outcome.Result.PromptVersion,
		Model:                 outcome.Result.Model,
		Disclaimer:            outcome.Result.Disclaimer,
		RunID:                 runID,
		Saved:                 saved,
	})
}

// resolveIntention maps an interface intention onto the words sent to the
// model.
//
// A known key keeps its existing label, so the interface's own vocabulary
// still works exactly as before. Anything else is the visitor's own text,
// bounded for abuse rather than for subject matter. Returning false means the
// request said nothing at all — the only thing that makes a discovery
// impossible to attempt.
func resolveIntention(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if label, known := intentionLabels[trimmed]; known {
		return label, true
	}
	if len([]rune(trimmed)) < minIntentionChars || len([]rune(trimmed)) > maxIntentionChars {
		return "", false
	}
	return trimmed, true
}

// newResearchProvider builds the research provider from the environment. A
// failure to configure one is a warning and a nil, never a refusal to start:
// a deployment with a bad search key should still serve discoveries that say
// they were not researched.
func newResearchProvider(log logger) research.Provider {
	provider, err := research.FromEnv(log)
	if err != nil {
		log.Warn("research provider is not configured; discoveries will be labelled un-researched",
			"reason", err.Error())
		return nil
	}
	return provider
}

// logger is the logging surface the research package needs, satisfied by
// *slog.Logger.
type logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

// gbifVerifier is a discovery.Verifier backed by the GBIF backbone taxonomy.
// It exists so the pipeline depends on an interface rather than on a concrete
// client, which is what lets its tests run without a network.
type gbifVerifier struct {
	client *research.GBIF
}

func (v gbifVerifier) VerifyIdentity(ctx context.Context, name string) (research.Identity, error) {
	if v.client == nil {
		return research.Identity{}, research.ErrNotConfigured
	}
	return v.client.VerifyIdentity(ctx, name)
}
