package research

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// SearchAPI is a research backend over a general web search service.
//
// The configured inference provider cannot search the web, so a deployment
// that wants true web search needs an external service. Rather than hard-code
// one vendor, this adapter speaks two of the common response shapes and
// detects which one it received, so the same code works against either.
//
// It is deliberately optional. The default deployment uses OpenWeb, which
// needs no key at all; this exists for the case where a project has a search
// subscription and wants broader coverage than two open archives give.
//
// The API key is read from the environment and never echoed, logged, returned
// in an error, or included in a source. A source's URL is whatever the search
// service reported, never a URL that embeds the query or the credential.
type SearchAPI struct {
	*baseBackend
	key string
}

// NewSearchAPI builds a backend against a search endpoint. A nil logger is
// replaced with one that discards.
func NewSearchAPI(endpoint, key string, logger Logger) *SearchAPI {
	if logger == nil {
		logger = nopLogger{}
	}
	return &SearchAPI{baseBackend: &baseBackend{
		name:    "search-api",
		baseURL: strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		logger:  logger,
		timeout: defaultTimeout,
	}, key: strings.TrimSpace(key)}
}

// Search implements Provider.
//
// The request is POSTed as JSON because that is what these services expect,
// and the response is probed for either known shape. A response matching
// neither is ErrBadResponse rather than an empty result: silently reporting
// "no results" for an unreadable reply would tell a visitor the web had
// nothing to say, which is a different and much stronger claim.
func (s *SearchAPI) Search(ctx context.Context, q Query, limit int) ([]Source, error) {
	terms := strings.TrimSpace(q.String())
	if terms == "" {
		return nil, ErrNoSources
	}
	if limit <= 0 {
		limit = 5
	}
	if s.baseURL == "" {
		return nil, ErrNotConfigured
	}

	target, err := url.Parse(s.baseURL)
	if err != nil || target.Host == "" {
		return nil, fmt.Errorf("%w: RESEARCH_API_URL is not a usable URL", ErrNotConfigured)
	}

	payload := map[string]any{
		"query": terms,
		"limit": limit,
	}
	// Tavily spells the count "max_results"; Serper and Brave use "num" and
	// "count". Sending the common key plus a couple of aliases is harmless and
	// keeps this working against services that ignore keys they do not know.
	payload["max_results"] = limit
	payload["num"] = limit
	payload["count"] = limit

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: encode request", ErrBadResponse)
	}

	raw, err := s.post(ctx, target, body)
	if err != nil {
		return nil, err
	}

	sources, err := decodeSearchResponse(raw)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	out := make([]Source, 0, len(sources))
	for _, r := range sources {
		title := cleanText(r.title, maxTitleChars)
		pageURL := strings.TrimSpace(r.url)
		if title == "" || !validURL(pageURL) {
			// A result with no address is not citable, and Everroot does not
			// show a visitor a source it cannot let them open.
			continue
		}
		publisher := hostOf(pageURL)
		out = append(out, Source{
			Title:       title,
			URL:         pageURL,
			Publisher:   publisher,
			Kind:        KindWebPage,
			Snippet:     cleanText(r.snippet, maxSnippetChars),
			RetrievedAt: now,
		})
		if len(out) == limit {
			break
		}
	}
	if len(out) == 0 {
		return nil, ErrNoSources
	}
	return out, nil
}

// post sends the search request and reads the bounded body.
func (s *SearchAPI) post(ctx context.Context, target *url.URL, payload []byte) ([]byte, error) {
	client := httpClient(s.timeout)
	return doPostJSON(ctx, client, target, payload, s.key)
}

// rawResult is one result in either supported shape.
type rawResult struct {
	title   string
	url     string
	snippet string
}

// searchResponse is the union of the shapes recognised.
//
// Tavily:  {"results":[{"title":..,"url":..,"content":..}]}
// Brave:   {"web":{"results":[{"title":..,"url":..,"description":..}]}}
// Serper:  {"organic":[{"title":..,"link":..,"snippet":..}]}
type searchResponse struct {
	Results []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Link        string `json:"link"`
		Content     string `json:"content"`
		Snippet     string `json:"snippet"`
		Description string `json:"description"`
	} `json:"results"`
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
	Organic []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
	} `json:"organic"`
}

// decodeSearchResponse probes each known shape in turn.
func decodeSearchResponse(raw []byte) ([]rawResult, error) {
	var parsed searchResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode search response", ErrBadResponse)
	}

	out := make([]rawResult, 0, 16)
	for _, r := range parsed.Results {
		out = append(out, rawResult{
			title:   r.Title,
			url:     firstNonBlank(r.URL, r.Link),
			snippet: firstNonBlank(r.Content, r.Snippet, r.Description),
		})
	}
	for _, r := range parsed.Web.Results {
		out = append(out, rawResult{title: r.Title, url: r.URL, snippet: r.Description})
	}
	for _, r := range parsed.Organic {
		out = append(out, rawResult{title: r.Title, url: r.Link, snippet: r.Snippet})
	}
	return out, nil
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// hostOf reduces a URL to its host for a publisher label. A search service
// rarely names the publisher, and inventing one would be a fabrication; the
// host is what the address actually says.
func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
}
