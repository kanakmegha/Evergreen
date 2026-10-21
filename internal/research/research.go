// Package research performs dynamic, source-attributed research for Everroot's
// discovery pipeline.
//
// It exists because a chat model's internal knowledge is not evidence. When a
// visitor asks about a tradition, a region or a species that Everroot has no
// reviewed record for, the honest answer has to come from somewhere real and be
// attributable to a URL — and the configured inference provider cannot search
// the web at all (its router rejects a "web_search" tool and offers no
// search-capable model). So research is a separate concern with its own
// provider interface, its own failure classification, and its own timeouts.
//
// Three rules shape everything here.
//
// Nothing is assumed about a backend. A provider either answered or it is
// unavailable; there is no third state in which an empty result silently means
// "we looked and found nothing" when the truth is "we could not look".
//
// Retrieved text is data, never instruction. A page a search engine returned
// can contain anything, including text addressed to an AI. Snippets are
// length-capped, control characters are stripped, and the prompt layer is
// responsible for the rest.
//
// A source is only as good as its address. Every source must carry a
// parseable http/https URL and a title, or it is dropped rather than passed on
// with a placeholder. Everroot never invents a URL, and a fabricated citation
// is worse than no citation.
package research

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Failure classes, mirroring the inference taxonomy so the HTTP layer can map
// either kind of failure onto the same error response.
var (
	// ErrNotConfigured means no research backend was configured. This is a
	// deployment fact, not a lookup that came back empty.
	ErrNotConfigured = errors.New("research provider is not configured")
	// ErrUnavailable means a configured backend could not be reached or
	// answered with a failure. It is never reported as "no results".
	ErrUnavailable = errors.New("research provider is unavailable")
	// ErrRateLimited means the backend asked us to slow down.
	ErrRateLimited = errors.New("research provider rate limited the request")
	// ErrTimeout means the backend did not answer inside the bound.
	ErrTimeout = errors.New("research request timed out")
	// ErrBadResponse means the backend answered with something unreadable.
	ErrBadResponse = errors.New("research provider returned an unusable response")
	// ErrNoSources means a backend answered successfully and genuinely found
	// nothing. This is distinct from ErrUnavailable on purpose.
	ErrNoSources = errors.New("research found no sources")
	// ErrPartial means some backends failed while others answered. The
	// returned sources are real, but the lookup was incomplete. A caller may
	// use the sources and must say the research was partial; it must not
	// report the reply as exhaustive.
	ErrPartial = errors.New("research was partial: at least one backend failed")
)

// Category is a safe, structured classification of a research failure.
type Category string

// Failure categories. They carry no host, path or query, so they are safe to
// log and to report to a visitor.
const (
	CategoryNotConfigured Category = "not_configured"
	CategoryUnavailable   Category = "unavailable"
	CategoryRateLimited   Category = "rate_limited"
	CategoryTimeout       Category = "timeout"
	CategoryBadResponse   Category = "bad_response"
	CategoryNoSources     Category = "no_sources"
	// CategoryPartial means some backends answered and some did not. The
	// findings are real; the search was incomplete.
	CategoryPartial Category = "partial"
	CategoryUnknown Category = "unknown"
)

// Diagnosis is a credential-free description of a research failure.
type Diagnosis struct {
	Category  Category
	Status    int
	Detail    string
	Retryable bool
}

// Kind classifies where a source came from, so the interface can distinguish a
// scholarly record from an encyclopedia entry without implying they carry the
// same weight.
type Kind string

// Source kinds Everroot recognises.
const (
	KindWebPage      Kind = "web_page"
	KindEncyclopedia Kind = "encyclopedia"
	KindBiodiversity Kind = "biodiversity_record"
	KindScholarly    Kind = "scholarly"
)

// Source is one attributable item of research.
//
// ID is assigned by the server and is what the model references when it cites
// a claim; the URL is what the visitor follows. Keeping them separate means a
// model cannot invent a citation by inventing an ID — an ID that does not
// belong to a real source is rejected downstream.
type Source struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Publisher   string    `json:"publisher,omitempty"`
	Kind        Kind      `json:"kind"`
	Snippet     string    `json:"snippet,omitempty"`
	RetrievedAt time.Time `json:"retrievedAt"`
}

// Query is one research question.
//
// Terms are what to search for. Species is optional and lets a biodiversity
// backend answer a botanical identity question directly rather than by
// guessing which search terms describe a binomial.
type Query struct {
	Terms   []string
	Species string
	// Region narrows a botanical question to a part of the world.
	Region string
}

// Strings joins the terms into one search string, collapsed and bounded.
func (q Query) String() string {
	parts := make([]string, 0, len(q.Terms)+2)
	for _, t := range q.Terms {
		if v := strings.TrimSpace(t); v != "" {
			parts = append(parts, v)
		}
	}
	if v := strings.TrimSpace(q.Species); v != "" {
		parts = append(parts, v)
	}
	return strings.Join(parts, " ")
}

// Provider performs dynamic research.
type Provider interface {
	// Name identifies the backend in logs and in the response's research
	// metadata. It is a constant, never derived from a request.
	Name() string
	// Search returns attributable sources for one query. Returning a nil slice
	// and a nil error means the backend answered and found nothing; returning
	// ErrNoSources says the same thing more explicitly. ErrUnavailable must
	// never be returned alongside results.
	Search(ctx context.Context, q Query, limit int) ([]Source, error)
}

// bounds shared by every backend. They exist so one hostile or enormous
// response cannot turn a discovery request into a memory problem.
const (
	// maxResponseBytes caps any single HTTP body read from a research backend.
	maxResponseBytes = 2 << 20
	// maxSnippetChars caps the extract carried into the model prompt. A
	// snippet is context for a claim, not a copy of the page.
	maxSnippetChars = 400
	// maxTitleChars caps a source title.
	maxTitleChars = 160
	// defaultTimeout bounds one backend call. Research runs inside a request
	// path, so it must be well inside the handler's own budget.
	defaultTimeout = 8 * time.Second
)

// httpClient is the shared transport for research backends. It is separate
// from the inference transport on purpose: a slow search host must not be able
// to consume the budget an inference call needs, and neither may inherit the
// other's retry behaviour.
func httpClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Redirects are allowed but bounded, and a redirect may not change
			// the scheme to something that is not http(s).
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}
}

// getJSON performs a bounded GET and returns the body. It classifies failures
// rather than returning raw transport errors, so callers never have to guess
// whether a failure is worth retrying or is terminal.
func getJSON(ctx context.Context, client *http.Client, target *url.URL) ([]byte, error) {
	if target.Scheme != "https" && target.Scheme != "http" {
		return nil, fmt.Errorf("%w: refusing non-http scheme", ErrUnavailable)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	// Identify the client honestly. These backends are open, and a
	// well-formed User-Agent is what they ask for.
	req.Header.Set("User-Agent", "Everroot/1.0 (tree discovery; research)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, classifyTransport(err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w: backend returned %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode >= 400:
		// A 4xx other than 429 is a request the backend rejected. That is a
		// bug in our query, not a backend outage, so it is not retryable.
		return nil, fmt.Errorf("%w: backend rejected the request (%d)", ErrBadResponse, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, classifyTransport(err)
	}
	return body, nil
}

// doPostJSON performs a bounded POST carrying a JSON body and returns the
// response body. It is used by the search-API backend, which — unlike the open
// archives — expects a POST and an API key.
//
// The key travels only in the Authorization header. It is never put in the URL,
// so it cannot surface in a redirect, a proxy log or an error string.
func doPostJSON(ctx context.Context, client *http.Client, target *url.URL, payload []byte, key string) ([]byte, error) {
	if target.Scheme != "https" && target.Scheme != "http" {
		return nil, fmt.Errorf("%w: refusing non-http scheme", ErrUnavailable)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	req.Header.Set("User-Agent", "Everroot/1.0 (tree discovery; research)")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, classifyTransport(err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		// A rejected key is a deployment problem, not a lookup that came back
		// empty. Reporting it as "no sources" would hide a broken credential.
		return nil, fmt.Errorf("%w: search service rejected the credentials (%d)",
			ErrNotConfigured, resp.StatusCode)
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w: backend returned %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("%w: backend rejected the request (%d)", ErrBadResponse, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, classifyTransport(err)
	}
	return body, nil
}

// classifyTransport maps a Go transport error onto the taxonomy. A timeout
// that arrives because our own context expired is reported as a timeout, not
// as an unavailable backend, because the two need different responses.
func classifyTransport(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	return fmt.Errorf("%w: %v", ErrUnavailable, err)
}

// Diagnose classifies a research failure for logging and for the error
// response. Only the category and a short detail are ever produced; the
// underlying URL is not carried, so nothing in a log line describes which
// backend was being called with which query.
func Diagnose(err error) Diagnosis {
	if err == nil {
		return Diagnosis{}
	}
	switch {
	case errors.Is(err, ErrNotConfigured):
		return Diagnosis{Category: CategoryNotConfigured}
	case errors.Is(err, ErrRateLimited):
		return Diagnosis{Category: CategoryRateLimited, Retryable: true}
	case errors.Is(err, ErrTimeout):
		return Diagnosis{Category: CategoryTimeout, Retryable: true}
	case errors.Is(err, ErrBadResponse):
		return Diagnosis{Category: CategoryBadResponse}
	case errors.Is(err, ErrNoSources):
		return Diagnosis{Category: CategoryNoSources}
	case errors.Is(err, ErrPartial):
		return Diagnosis{Category: CategoryPartial, Retryable: true}
	case errors.Is(err, ErrUnavailable):
		return Diagnosis{Category: CategoryUnavailable, Retryable: true}
	}
	return Diagnosis{Category: CategoryUnknown}
}

// cleanText strips control characters and collapses whitespace. Retrieved text
// arrives from anywhere on the internet; nothing here lets a stray byte
// sequence reach the model or the interface.
func cleanText(s string, max int) string {
	var b strings.Builder
	b.Grow(min(len(s), max))
	lastSpace := false
	for _, r := range s {
		switch r {
		case '\n', '\t', '\r', ' ', ' ', '　':
			// Whitespace becomes a single space. A newline inside a JSON
			// string value is a hazard rather than structure, but deleting it
			// outright would weld two words together, so it is normalised
			// rather than removed.
			if lastSpace {
				continue
			}
			lastSpace = true
			b.WriteRune(' ')
			continue
		}
		if r < 0x20 || r == 0x7f {
			// Every other control character is dropped. None of them are
			// meaningful in a snippet, and some are exactly what an attacker
			// would want to smuggle past a naive length check.
			continue
		}
		lastSpace = false
		b.WriteRune(r)
		if b.Len() >= max {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

// validURL reports whether a string is a usable citation target. Only absolute
// http(s) URLs are accepted: a relative reference, a javascript: URL or a
// data: URL is never something Everroot will show a visitor as a source.
func validURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}
	if u.Host == "" {
		return false
	}
	return true
}
