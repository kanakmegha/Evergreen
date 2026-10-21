package research

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// stubDoer answers with a fixed body, so a backend test never touches the
// network and never depends on a live archive's uptime.
type stubDoer struct {
	body string
	err  error
	// seen records the URL the backend actually requested, which is how a
	// test proves a query travelled at all.
	seen *url.URL
}

func (s *stubDoer) GetJSON(_ context.Context, u *url.URL) ([]byte, error) {
	if s.seen != nil {
		*s.seen = *u
	}
	if s.err != nil {
		return nil, s.err
	}
	return []byte(s.body), nil
}

func TestWikipediaSearchBuildsAQueryAndReadsResults(t *testing.T) {
	var seen url.URL
	w := NewWikipedia(nil)
	w.http = &stubDoer{
		seen: &seen,
		body: `{"query":{"pages":[
			{"pageid":10,"title":"Ashoka tree","extract":"A tree of the Indian subcontinent.","fullurl":"https://en.wikipedia.org/wiki/Ashoka_tree","index":1},
			{"pageid":20,"title":"Saraca asoca","extract":"An evergreen tree in the family Fabaceae.","fullurl":"https://en.wikipedia.org/wiki/Saraca_asoca","index":2},
			{"pageid":30,"title":"No extract","fullurl":"https://en.wikipedia.org/wiki/No_extract","index":3}
		]}}`,
	}

	sources, err := w.Search(context.Background(), Query{Terms: []string{"Ashoka", "tree"}}, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The page with no extract is dropped: a citation with nothing behind it
	// is worse than no citation.
	if len(sources) != 2 {
		t.Fatalf("got %d sources, want 2 (the extractless page must be dropped)", len(sources))
	}
	if got := sources[0].Title; got != "Ashoka tree" {
		t.Errorf("first source = %q, want relevance-ordered %q", got, "Ashoka tree")
	}
	for _, s := range sources {
		if s.Publisher != "Wikipedia" {
			t.Errorf("publisher = %q, want Wikipedia", s.Publisher)
		}
		if s.Kind != KindEncyclopedia {
			t.Errorf("kind = %q, want %q: an encyclopedia entry must not read as a scholarly source", s.Kind, KindEncyclopedia)
		}
		if !strings.HasPrefix(s.URL, "https://en.wikipedia.org/wiki/") {
			t.Errorf("url = %q, want a wikipedia page URL", s.URL)
		}
		if s.RetrievedAt.IsZero() {
			t.Error("RetrievedAt must be set so the citation carries a retrieval time")
		}
	}

	if q := seen.Query().Get("gsrsearch"); q != "Ashoka tree" {
		t.Errorf("search term = %q, want the terms joined as one query", q)
	}
	if got := seen.Query().Get("explaintext"); got != "1" {
		t.Errorf("explaintext = %q, want plain-text extracts", got)
	}
}

func TestWikipediaRejectsAnUnreadableReply(t *testing.T) {
	w := NewWikipedia(nil)
	w.http = &stubDoer{body: `<html>not json</html>`}

	_, err := w.Search(context.Background(), Query{Terms: []string{"x"}}, 5)
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err = %v, want ErrBadResponse; an unreadable reply must never read as 'no results'", err)
	}
}

func TestWikipediaEmptyResultIsNotAFailure(t *testing.T) {
	w := NewWikipedia(nil)
	w.http = &stubDoer{body: `{"query":{"pages":[]}}`}

	sources, err := w.Search(context.Background(), Query{Terms: []string{"zzzzunlikely"}}, 5)
	if !errors.Is(err, ErrNoSources) {
		t.Fatalf("err = %v, want ErrNoSources", err)
	}
	if sources != nil {
		t.Errorf("sources = %v, want nil", sources)
	}
}

func TestWikipediaRefusesABackendFailureDisguisedAsEmpty(t *testing.T) {
	w := NewWikipedia(nil)
	w.http = &stubDoer{err: ErrUnavailable}

	_, err := w.Search(context.Background(), Query{Terms: []string{"x"}}, 5)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if errors.Is(err, ErrNoSources) {
		t.Fatal("a transport failure must never be reported as 'found nothing'")
	}
}

func TestGBIFVerifiesAnAcceptedName(t *testing.T) {
	var seen url.URL
	g := NewGBIF(nil)
	g.http = &stubDoer{
		seen: &seen,
		body: `{"results":[
			{"key":5023541,"scientificName":"Saraca asoca (Roxb.) Dekker","canonicalName":"Saraca asoca","rank":"SPECIES","taxonomicStatus":"ACCEPTED","family":"Fabaceae"},
			{"key":1,"scientificName":"Something else","canonicalName":"Other tree","rank":"SPECIES","taxonomicStatus":"ACCEPTED","family":"Otheraceae"}
		]}`,
	}

	identity, err := g.VerifyIdentity(context.Background(), "Saraca asoca")
	if err != nil {
		t.Fatalf("VerifyIdentity: %v", err)
	}
	if !identity.Matched {
		t.Fatal("Matched = false, want true")
	}
	if identity.Family != "Fabaceae" {
		t.Errorf("Family = %q, want Fabaceae", identity.Family)
	}
	if identity.Status != "ACCEPTED" {
		t.Errorf("Status = %q, want ACCEPTED", identity.Status)
	}
	if want := "https://www.gbif.org/species/5023541"; identity.Source != want {
		t.Errorf("Source = %q, want %q", identity.Source, want)
	}
	if got := seen.Query().Get("status"); got != "ACCEPTED" {
		t.Errorf("status filter = %q, want ACCEPTED", got)
	}
}

func TestGBIFDoesNotAcceptANearMissAsVerified(t *testing.T) {
	g := NewGBIF(nil)
	// The only hit is a different species entirely. Reporting it as a match
	// would verify a name GBIF never confirmed.
	g.http = &stubDoer{body: `{"results":[
		{"key":1,"scientificName":"Ficus benghalensis","canonicalName":"Ficus benghalensis","rank":"SPECIES","taxonomicStatus":"ACCEPTED","family":"Moraceae"}
	]}`}

	identity, err := g.VerifyIdentity(context.Background(), "Saraca asoca")
	if err != nil {
		t.Fatalf("VerifyIdentity: %v", err)
	}
	if identity.Matched {
		t.Error("Matched = true for a different species; an approximate hit must not verify a name")
	}
	if identity.ScientificName != "" || identity.Source != "" {
		t.Errorf("unmatched identity must carry no name or source, got %+v", identity)
	}
}

func TestGBIFLookupFailureIsNotAnUnmatchedName(t *testing.T) {
	g := NewGBIF(nil)
	g.http = &stubDoer{err: ErrTimeout}

	_, err := g.VerifyIdentity(context.Background(), "Saraca asoca")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout; a timeout must not read as 'GBIF has no record'", err)
	}
}

func TestGBIFSearchNeedsASpecies(t *testing.T) {
	g := NewGBIF(nil)
	_, err := g.Search(context.Background(), Query{Terms: []string{"festival", "trees"}}, 5)
	if !errors.Is(err, ErrNoSources) {
		t.Fatalf("err = %v, want ErrNoSources; GBIF answers identity questions, not free-text ones", err)
	}
}

func TestMultiMergesAndDeduplicates(t *testing.T) {
	now := time.Now()
	dup := Source{Title: "Same", URL: "https://example.org/a", RetrievedAt: now}
	other := Source{Title: "Other", URL: "https://example.org/b", RetrievedAt: now}
	m := Multi{Providers: []Provider{
		fixedProvider{"one", []Source{dup, other}, nil},
		fixedProvider{"two", []Source{dup}, nil},
	}}

	sources, err := m.Search(context.Background(), Query{Terms: []string{"x"}}, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("got %d sources, want 2 after de-duplication", len(sources))
	}
}

func TestMultiSurvivesOneBackendFailing(t *testing.T) {
	good := Source{Title: "Good", URL: "https://example.org/good", RetrievedAt: time.Now()}
	m := Multi{Providers: []Provider{
		fixedProvider{"broken", nil, ErrUnavailable},
		fixedProvider{"working", []Source{good}, nil},
	}}

	sources, err := m.Search(context.Background(), Query{Terms: []string{"x"}}, 10)
	if !errors.Is(err, ErrPartial) {
		t.Fatalf("err = %v, want ErrPartial; a partly failed search must say so", err)
	}
	// The findings survive. Reporting ErrPartial alongside them is what lets a
	// caller ground its answer and still state the research was incomplete.
	if len(sources) != 1 || sources[0].Title != "Good" {
		t.Fatalf("sources = %+v, want the surviving backend's finding", sources)
	}
}

func TestMultiReportsUnavailableWhenEveryBackendFails(t *testing.T) {
	m := Multi{Providers: []Provider{
		fixedProvider{"a", nil, ErrUnavailable},
		fixedProvider{"b", nil, ErrTimeout},
	}}

	_, err := m.Search(context.Background(), Query{Terms: []string{"x"}}, 10)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable; a total outage is not an empty answer", err)
	}
}

func TestMultiTellsAFullOutageApartFromAnEmptyAnswer(t *testing.T) {
	m := Multi{Providers: []Provider{
		fixedProvider{"a", nil, ErrNoSources},
		fixedProvider{"b", nil, ErrNoSources},
	}}

	_, err := m.Search(context.Background(), Query{Terms: []string{"x"}}, 10)
	if !errors.Is(err, ErrNoSources) {
		t.Fatalf("err = %v, want ErrNoSources; backends that answered must not be reported as an outage", err)
	}
}

func TestNoneIsNeverAnAnswer(t *testing.T) {
	_, err := None{}.Search(context.Background(), Query{Terms: []string{"x"}}, 5)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured; an unconfigured deployment must not pretend to have researched", err)
	}
}

func TestSearchAPIReadsATavilyShapedReply(t *testing.T) {
	var gotAuth string
	var posted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		posted = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"title":"Ficus benghalensis","url":"https://example.org/banyan","content":"The banyan is a strangler fig."},
			{"title":"No address","content":"not citable"}
		]}`))
	}))
	defer server.Close()

	s := NewSearchAPI(server.URL, "secret-key", nil)
	sources, err := s.Search(context.Background(), Query{Terms: []string{"banyan"}}, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The address-less result is dropped: Everroot never shows a visitor a
	// source it cannot let them open.
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1 (the address-less result must be dropped)", len(sources))
	}
	if sources[0].URL != "https://example.org/banyan" {
		t.Errorf("url = %q", sources[0].URL)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization = %q, want the key in the header only", gotAuth)
	}
	if !posted {
		t.Error("the backend never posted to the configured endpoint")
	}
	if strings.Contains(sources[0].URL, "secret-key") {
		t.Fatal("the API key leaked into a source URL")
	}
}

func TestSearchAPIReportsRejectedCredentialsAsConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	s := NewSearchAPI(server.URL, "bad", nil)
	_, err := s.Search(context.Background(), Query{Terms: []string{"x"}}, 5)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured; a rejected key is a deployment fault, not an empty search", err)
	}
	if errors.Is(err, ErrNoSources) {
		t.Fatal("a rejected credential must never be reported as 'no results'")
	}
}

func TestSearchAPIReadsASerperShapedReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"organic":[
			{"title":"Saraca asoca","link":"https://example.org/asoca","snippet":"An evergreen tree."}
		]}`))
	}))
	defer server.Close()

	s := NewSearchAPI(server.URL, "", nil)
	sources, err := s.Search(context.Background(), Query{Terms: []string{"asoca"}}, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(sources) != 1 || sources[0].URL != "https://example.org/asoca" {
		t.Fatalf("sources = %+v, want the Serper-shaped result", sources)
	}
	if sources[0].Publisher != "example.org" {
		t.Errorf("publisher = %q, want the host of the source", sources[0].Publisher)
	}
}

func TestFromEnvReturnsNothingWhenUnconfigured(t *testing.T) {
	t.Setenv("RESEARCH_PROVIDER", "")
	p, err := FromEnv(nil)
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if p != nil {
		t.Fatalf("provider = %v, want nil when nothing is configured", p)
	}
}

func TestFromEnvRejectsASearchAPIWithoutAnEndpoint(t *testing.T) {
	t.Setenv("RESEARCH_PROVIDER", "search-api")
	t.Setenv("RESEARCH_API_URL", "")
	if _, err := FromEnv(nil); err == nil {
		t.Fatal("want an error: a search backend with no endpoint would silently never run")
	}
}

func TestFromEnvRejectsAnUnknownProvider(t *testing.T) {
	t.Setenv("RESEARCH_PROVIDER", "not-a-provider")
	if _, err := FromEnv(nil); err == nil {
		t.Fatal("want an error for an unrecognised provider name")
	}
}

func TestValidURLRejectsAnythingThatIsNotHTTP(t *testing.T) {
	for _, raw := range []string{
		"javascript:alert(1)",
		"data:text/html,<script>",
		"/relative/path",
		"ftp://example.org/file",
		"",
		"https://",
	} {
		if validURL(raw) {
			t.Errorf("validURL(%q) = true, want false", raw)
		}
	}
	if !validURL("https://example.org/page") {
		t.Error("validURL(https://example.org/page) = false, want true")
	}
}

func TestCleanTextStripsControlCharactersAndCapsLength(t *testing.T) {
	in := "line one\nline two\twith   spaces"
	got := cleanText(in, 100)
	if strings.ContainsAny(got, "\n\t\x00\x07") {
		t.Errorf("cleanText left a control character in %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Errorf("cleanText left a double space in %q", got)
	}
	if got != "line one line two with spaces" {
		t.Errorf("cleanText = %q", got)
	}

	long := cleanText(strings.Repeat("a", 500), 20)
	if len([]rune(long)) > 20 {
		t.Errorf("cleanText returned %d runes for a 20 budget", len([]rune(long)))
	}
}

// TestRetrievedTextIsTreatedAsData proves the property the whole package
// rests on: a snippet is capped and stripped, so nothing arriving from a page
// can grow without bound or smuggle a control character into the prompt.
func TestRetrievedTextIsTreatedAsData(t *testing.T) {
	hostile := strings.Repeat("<script>alert(1)</script> ", 1000)
	got := cleanText(hostile, maxSnippetChars)
	if len([]rune(got)) > maxSnippetChars {
		t.Errorf("snippet is %d runes, budget is %d", len([]rune(got)), maxSnippetChars)
	}
}

func TestDiagnoseClassifiesEveryFailure(t *testing.T) {
	cases := []struct {
		err  error
		want Category
	}{
		{ErrNotConfigured, CategoryNotConfigured},
		{ErrRateLimited, CategoryRateLimited},
		{ErrTimeout, CategoryTimeout},
		{ErrBadResponse, CategoryBadResponse},
		{ErrNoSources, CategoryNoSources},
		{ErrUnavailable, CategoryUnavailable},
		{errors.New("something else"), CategoryUnknown},
	}
	for _, tc := range cases {
		if got := Diagnose(tc.err).Category; got != tc.want {
			t.Errorf("Diagnose(%v).Category = %q, want %q", tc.err, got, tc.want)
		}
	}
	if Diagnose(nil).Category != "" {
		t.Error("Diagnose(nil) must classify as no failure")
	}
}

// fixedProvider is a Provider test double.
type fixedProvider struct {
	name    string
	sources []Source
	err     error
}

func (f fixedProvider) Name() string { return f.name }
func (f fixedProvider) Search(context.Context, Query, int) ([]Source, error) {
	return f.sources, f.err
}
