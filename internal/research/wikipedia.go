package research

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Logger is the minimal logging surface this package needs. It is declared
// here rather than imported so a caller can pass its own logger without this
// package depending on one.
//
// Nothing this package logs contains a query, a snippet or a credential: a
// research query is derived from a visitor's own words, and a backend URL
// identifies a host rather than a secret.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any) {}
func (nopLogger) Warn(string, ...any) {}

// httpDoer is the small slice of an HTTP client this package's backends need.
// Naming it keeps the fake used by the tests obvious, and keeps a backend from
// reaching for anything else on the client.
type httpDoer interface {
	GetJSON(ctx context.Context, u *url.URL) ([]byte, error)
}

// doerFunc adapts a plain function to httpDoer.
type doerFunc func(ctx context.Context, u *url.URL) ([]byte, error)

// GetJSON implements httpDoer.
func (f doerFunc) GetJSON(ctx context.Context, u *url.URL) ([]byte, error) { return f(ctx, u) }

// doer returns the backend's HTTP client, building the real one on first use.
// Building lazily means constructing a provider at start-up never touches the
// network, and a test can install a fake before the first call.
func (b *baseBackend) doer() httpDoer {
	if b.http != nil {
		return b.http
	}
	c := httpClient(b.timeout)
	return doerFunc(func(ctx context.Context, u *url.URL) ([]byte, error) {
		return getJSON(ctx, c, u)
	})
}

// baseBackend holds what every HTTP-backed research provider shares: an
// endpoint, a client, a logger and a timeout.
type baseBackend struct {
	name    string
	baseURL string
	http    httpDoer
	logger  Logger
	timeout time.Duration
}

// Name implements Provider.
func (b *baseBackend) Name() string { return b.name }

// WikipediaBaseURL is the MediaWiki API endpoint. It is a variable so a test
// can point it at a stub server.
var WikipediaBaseURL = "https://en.wikipedia.org"

// Wikipedia is a research backend over the MediaWiki API.
//
// It is here because it is the one open, key-free, stable source that covers
// exactly the questions Everroot gets asked and cannot answer from its own
// records: what a festival is, which trees a region actually grows, what a
// tradition practises. It is not authoritative in the way a peer-reviewed paper
// is, and the source kind carries that — KindEncyclopedia — so nothing Everroot
// renders implies more weight than an encyclopedia entry deserves.
//
// One request does the whole job. The generator form returns the search hits
// with a plain-text intro extract and the canonical page URL together, which
// keeps a lookup to a single round trip and removes the failure mode where a
// title is found but its extract is not.
type Wikipedia struct {
	*baseBackend
}

// NewWikipedia builds a Wikipedia backend. A nil logger is replaced with one
// that discards.
func NewWikipedia(logger Logger) *Wikipedia {
	if logger == nil {
		logger = nopLogger{}
	}
	return &Wikipedia{&baseBackend{
		name:    "wikipedia",
		baseURL: WikipediaBaseURL,
		logger:  logger,
		timeout: defaultTimeout,
	}}
}

// wikiQuery is the shape the MediaWiki generator form returns. Only the fields
// Everroot uses are decoded; everything else is ignored on purpose, so a new
// field in a third-party response cannot break a lookup.
type wikiQuery struct {
	Query struct {
		Pages []wikiPage `json:"pages"`
	} `json:"query"`
}

type wikiPage struct {
	PageID  int    `json:"pageid"`
	Title   string `json:"title"`
	Extract string `json:"extract"`
	FullURL string `json:"fullurl"`
	Index   int    `json:"index"`
}

// Search implements Provider.
func (w *Wikipedia) Search(ctx context.Context, q Query, limit int) ([]Source, error) {
	terms := strings.TrimSpace(q.String())
	if terms == "" {
		return nil, ErrNoSources
	}
	if limit <= 0 {
		limit = 5
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("generator", "search")
	params.Set("gsrsearch", terms)
	params.Set("gsrlimit", fmt.Sprint(limit))
	params.Set("gsrnamespace", "0")
	params.Set("prop", "extracts|info")
	params.Set("exintro", "1")
	params.Set("explaintext", "1")
	params.Set("exsentences", "3")
	params.Set("inprop", "url")
	params.Set("redirects", "1")
	params.Set("format", "json")
	params.Set("formatversion", "2")

	target, err := url.Parse(w.baseURL + "/w/api.php?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("%w: build query", ErrBadResponse)
	}

	body, err := w.doer().GetJSON(ctx, target)
	if err != nil {
		return nil, err
	}

	var parsed wikiQuery
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode search response", ErrBadResponse)
	}

	now := time.Now().UTC()
	sources := make([]Source, 0, len(parsed.Query.Pages))
	for _, page := range parsed.Query.Pages {
		// A page with no extract is a disambiguation or stub page; carrying it
		// would put a citation in front of the model with nothing behind it.
		extract := cleanText(page.Extract, maxSnippetChars)
		pageURL := strings.TrimSpace(page.FullURL)
		title := cleanText(page.Title, maxTitleChars)
		if extract == "" || title == "" || !validURL(pageURL) {
			continue
		}
		sources = append(sources, Source{
			Title:       title,
			URL:         pageURL,
			Publisher:   "Wikipedia",
			Kind:        KindEncyclopedia,
			Snippet:     extract,
			RetrievedAt: now,
			ID:          fmt.Sprint(page.PageID),
		})
	}

	if len(sources) == 0 {
		return nil, ErrNoSources
	}
	// The generator's relevance order is the page's "index" field. Sorting by
	// it keeps a deterministic order so the same query yields the same list.
	sortByRelevance(sources)
	if len(sources) > limit {
		sources = sources[:limit]
	}
	return sources, nil
}

// sortByRelevance orders sources by their MediaWiki relevance index, which was
// carried in the ID field as a decimal string. Sources without a parseable
// index keep their relative order at the end.
func sortByRelevance(sources []Source) {
	rank := func(s Source) int {
		var n int
		if _, err := fmt.Sscanf(s.ID, "%d", &n); err != nil {
			return 1 << 30
		}
		return n
	}
	// Insertion sort: the lists here are a handful of items, and this keeps
	// the order stable without a comparator that has to be reasoned about.
	for i := 1; i < len(sources); i++ {
		for j := i; j > 0 && rank(sources[j]) < rank(sources[j-1]); j-- {
			sources[j], sources[j-1] = sources[j-1], sources[j]
		}
	}
}
