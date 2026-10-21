package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"everroot/internal/discovery"
	"everroot/internal/store"
)

// bearerToken reads the Authorization header.
//
// Only the Bearer scheme is accepted, and a missing or malformed header is
// reported as "no credentials" rather than as an error: most callers are
// anonymous, and treating that as a fault would put an error banner in front
// of a visitor who did nothing wrong.
func bearerToken(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return "", false
	}
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// ownerFrom resolves the caller's bearer token to an owner.
//
// It reports three states rather than two, because they need three different
// responses: no credentials (the caller is anonymous and may read nothing),
// invalid credentials (the caller presented a token that is not usable, and
// should be told so their client can obtain a new one), and a database that
// could not be consulted at all.
func (a *API) ownerFrom(r *http.Request) (store.Owner, bool, error) {
	if a.store == nil {
		return store.Owner{}, false, store.ErrUnavailable
	}
	token, ok := bearerToken(r)
	if !ok {
		return store.Owner{}, false, nil
	}
	owner, err := a.store.Authorize(r.Context(), token)
	if err != nil {
		return store.Owner{}, false, err
	}
	return owner, true, nil
}

// writeStoreError maps a store failure onto the response a caller should see.
//
// Nothing here carries the store's own error text outward: a pgx error can
// embed the host and the user from the DSN, and a visitor has no use for it
// either way.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "That session token is no longer valid. Start a new session to continue.")
	case errors.Is(err, store.ErrForbidden):
		// Deliberately identical to a missing run, so an id cannot be used to
		// discover which discoveries exist.
		writeError(w, http.StatusNotFound, "That discovery is not available.")
	case errors.Is(err, store.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "Saving discoveries is temporarily unavailable. Your answer is still shown above.")
	default:
		writeError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
	}
}

// sessionRequest is the ask for a session token.
type sessionRequest struct {
	// Handle is an optional opaque label the visitor chose. It is not an
	// identity, is never used to authorise anything, and Everroot does not ask
	// for an email, a name or a date of birth anywhere in this flow.
	Handle string `json:"handle"`
}

// sessionResponse returns a freshly issued token.
type sessionResponse struct {
	Token string `json:"token"`
	// ExpiresAt tells the client when to obtain a new token, so a visitor is
	// never surprised by a session that lapsed without warning.
	ExpiresAt string `json:"expiresAt"`
	// Anonymous records that this is an ordinary visitor session. Everroot
	// offers account sign-in as an addition to this, never as a requirement.
	Anonymous bool `json:"anonymous"`
}

// createSession handles POST /api/v1/session.
//
// A session is how a visitor keeps their own discoveries readable later
// without Everroot knowing who they are. The token is returned exactly once
// and is not recoverable afterwards; only its hash is stored.
func (a *API) createSession(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeStoreError(w, store.ErrUnavailable)
		return
	}
	var req sessionRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, 400, "invalid JSON request")
			return
		}
	}
	if len([]rune(req.Handle)) > 60 {
		writeError(w, 400, "handle is too long")
		return
	}

	token, _, err := a.store.NewAnonymousToken(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	expiresAt, err := a.store.TokenExpiry(r.Context(), token)
	if err != nil {
		// The token was issued and is usable; only its expiry could not be
		// read back. Reporting a session without a date beats failing a
		// request that already succeeded at the thing that mattered.
		a.log.Warn("could not read token expiry", "error", err)
	}
	writeJSON(w, 201, sessionResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		Anonymous: true,
	})
}

// listDiscoveries handles GET /api/v1/discovery.
//
// It returns only the caller's own runs. There is no query parameter that can
// widen that, and no id that can be supplied to reach someone else's.
func (a *API) listDiscoveries(w http.ResponseWriter, r *http.Request) {
	owner, ok, err := a.ownerFrom(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "Start a session to see your saved discoveries.")
		return
	}

	runs, err := a.store.List(r.Context(), owner, 20)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if runs == nil {
		runs = []store.RunSummary{}
	}
	writeJSON(w, 200, map[string]any{"discoveries": runs})
}

// getDiscovery handles GET /api/v1/discovery/{id}.
func (a *API) getDiscovery(w http.ResponseWriter, r *http.Request) {
	owner, ok, err := a.ownerFrom(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "Start a session to read your saved discoveries.")
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 64 {
		writeError(w, http.StatusNotFound, "That discovery is not available.")
		return
	}

	run, err := a.store.Get(r.Context(), owner, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, 200, run)
}

// deleteDiscovery handles DELETE /api/v1/discovery/{id}.
func (a *API) deleteDiscovery(w http.ResponseWriter, r *http.Request) {
	owner, ok, err := a.ownerFrom(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "Start a session to manage your saved discoveries.")
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 64 {
		writeError(w, http.StatusNotFound, "That discovery is not available.")
		return
	}

	if err := a.store.Delete(r.Context(), owner, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// saveOutcome persists a completed discovery when the caller presented a
// session token.
//
// A failure to save is never allowed to cost the visitor the answer they
// already have. The pipeline spent a model call and a research lookup to
// produce it; discarding that because a write failed would be the wrong trade.
// Instead the response carries a flag saying the save did not happen, so the
// interface can say so plainly rather than leaving a visitor to assume their
// discovery was kept.
func (a *API) saveOutcome(ctx context.Context, r *http.Request, outcome discovery.Outcome, req aiDiscoveryRequest) (runID string, saved bool) {
	if a.store == nil || outcome.Err != nil {
		return "", false
	}
	owner, ok, err := a.ownerFrom(r)
	if err != nil || !ok {
		// No usable token. That is the normal case for a first-time visitor,
		// and it is not an error worth logging.
		return "", false
	}

	sources := make([]store.SavedSource, 0, len(outcome.Sources))
	for _, s := range outcome.Sources {
		sources = append(sources, store.SavedSource{
			SourceID:    s.ID,
			Title:       s.Title,
			URL:         s.URL,
			Publisher:   s.Publisher,
			Kind:        string(s.Kind),
			Snippet:     s.Snippet,
			RetrievedAt: s.RetrievedAt,
		})
	}

	id, err := a.store.Save(ctx, owner, store.SavedRun{
		Intention:      req.Intention,
		Description:    req.Description,
		Tradition:      req.Tradition,
		Region:         req.Region,
		Nakshatra:      req.Nakshatra,
		Constraints:    req.Constraints,
		ResearchStatus: outcome.Status,
		PromptVersion:  outcome.Result.PromptVersion,
		Model:          outcome.Result.Model,
		Disclaimer:     outcome.Result.Disclaimer,
		Payload:        outcome.Result,
		Sources:        sources,
		FollowUp:       outcome.Result.FollowUpQuestion,
	})
	if err != nil {
		// Logged, not surfaced: the visitor keeps their answer either way.
		a.log.Warn("could not save a discovery", "category", "store_unavailable")
		return "", false
	}
	return id, true
}
