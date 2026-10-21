package research

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// None is the explicit "research is switched off" provider.
//
// It exists so that "no research backend configured" is a state the
// application can name. Without it, a deployment that never set RESEARCH_PROVIDER
// would answer a visitor with findings that were never researched — which is
// the exact failure this package is built to prevent.
type None struct{}

// Name implements Provider.
func (None) Name() string { return "none" }

// Search implements Provider. It always reports ErrNotConfigured, because
// reporting an empty result set would say "we researched and found nothing"
// when the truth is that nothing was researched.
func (None) Search(context.Context, Query, int) ([]Source, error) {
	return nil, ErrNotConfigured
}

// Multi fans one query out across several providers and merges the results.
//
// A backend that fails is not fatal: the reply still carries whatever the other
// backends returned, and the failure is reported through Partial so the
// interface can say research was incomplete rather than implying it was
// exhaustive. If every backend fails, the whole call fails.
type Multi struct {
	Providers []Provider
	// PerProvider bounds how long any single backend may take. Backends run
	// concurrently, so the fan-out costs one PerProvider, not the sum.
	PerProvider time.Duration
}

// Name implements Provider.
func (m Multi) Name() string {
	names := make([]string, 0, len(m.Providers))
	for _, p := range m.Providers {
		names = append(names, p.Name())
	}
	return "multi(" + strings.Join(names, ",") + ")"
}

// backendResult carries one backend's answer back to the merger, together with
// the failure if there was one. The error is kept rather than logged so the
// caller can decide what a partial answer is worth.
type backendResult struct {
	name    string
	sources []Source
	err     error
}

// Search implements Provider.
func (m Multi) Search(ctx context.Context, q Query, limit int) ([]Source, error) {
	if len(m.Providers) == 0 {
		return nil, ErrNotConfigured
	}
	per := m.PerProvider
	if per <= 0 {
		per = defaultTimeout
	}

	results := make(chan backendResult, len(m.Providers))
	var wg sync.WaitGroup
	for _, p := range m.Providers {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, per)
			defer cancel()
			// Each backend gets its own share of the total so one slow host
			// cannot consume the whole budget on its own.
			sources, err := p.Search(pctx, q, limit)
			results <- backendResult{name: p.Name(), sources: sources, err: err}
		}(p)
	}
	wg.Wait()
	close(results)

	var merged []Source
	seen := map[string]bool{}
	var failures []string
	for r := range results {
		if r.err != nil {
			// A backend that answered and found nothing is not a failure.
			if !errors.Is(r.err, ErrNoSources) {
				failures = append(failures, r.name)
			}
			continue
		}
		for _, s := range r.sources {
			key := sourceKey(s)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, s)
		}
	}

	if len(merged) == 0 {
		if len(failures) > 0 {
			return nil, fmt.Errorf("%w: every research backend failed (%d of %d)",
				ErrUnavailable, len(failures), len(m.Providers))
		}
		return nil, ErrNoSources
	}
	// Prefer sources from backends that actually answered; the caller asked for
	// `limit` and a stable order matters for reproducible tests.
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].RetrievedAt.Before(merged[j].RetrievedAt) })
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	if len(failures) > 0 {
		// Some backends answered and some did not. Returning the findings
		// alongside ErrPartial lets a caller ground its answer while stating
		// honestly that the research was incomplete — which is a different
		// thing from an outage and a different thing from an exhaustive
		// search.
		return merged, fmt.Errorf("%w (%d of %d backends failed)",
			ErrPartial, len(failures), len(m.Providers))
	}
	return merged, nil
}

// sourceKey is the de-duplication key. The host-agnostic path is used so that
// the same article reached over two slightly different URLs collapses into one
// source rather than appearing twice.
func sourceKey(s Source) string {
	u := strings.TrimRight(strings.ToLower(strings.TrimSpace(s.URL)), "/")
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "www.")
	if u == "" {
		return ""
	}
	return u
}

// FromEnv builds the provider the deployment configured, or nil when none is
// configured.
//
// It reads RESEARCH_PROVIDER (or SEARCH_PROVIDER), RESEARCH_API_URL (or SEARCH_API_URL),
// and RESEARCH_API_KEY (or SEARCH_API_KEY). The key is used and never returned, logged or
// included in any error.
//
// The recognised values are:
//
//	"" or "none"   research is off; discovery runs without sources
//	"open-web"     Wikipedia plus GBIF, no API key required (the default when
//	               a key-free deployment wants research)
//	"search-api"   a search API spoken over RESEARCH_API_URL/RESEARCH_API_KEY
//	"open-web,search-api"  both, fanned out concurrently
func FromEnv(logger Logger) (Provider, error) {
	name := strings.TrimSpace(os.Getenv("RESEARCH_PROVIDER"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("SEARCH_PROVIDER"))
	}
	apiURL := strings.TrimSpace(os.Getenv("RESEARCH_API_URL"))
	if apiURL == "" {
		apiURL = strings.TrimSpace(os.Getenv("SEARCH_API_URL"))
	}
	apiKey := strings.TrimSpace(os.Getenv("RESEARCH_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("SEARCH_API_KEY"))
	}

	switch strings.ToLower(name) {
	case "none", "off", "disabled":
		return None{}, nil
	case "open-web":
		return OpenWeb(logger), nil
	case "search-api":
		if apiURL == "" {
			return nil, errors.New("RESEARCH_PROVIDER=search-api requires RESEARCH_API_URL")
		}
		return NewSearchAPI(apiURL, apiKey, logger), nil
	case "":
		if apiURL != "" {
			return NewSearchAPI(apiURL, apiKey, logger), nil
		}
		return nil, nil
	}
	if strings.Contains(strings.ToLower(name), ",") {
		var providers []Provider
		for _, part := range strings.Split(name, ",") {
			p, err := FromEnvNamed(strings.TrimSpace(part), logger)
			if err != nil {
				return nil, err
			}
			if p != nil {
				providers = append(providers, p)
			}
		}
		if len(providers) == 0 {
			return nil, nil
		}
		return Multi{Providers: providers}, nil
	}
	return nil, fmt.Errorf("unrecognised RESEARCH_PROVIDER %q", name)
}

// FromEnvNamed resolves a single provider name, for the multi-backend case.
func FromEnvNamed(name string, logger Logger) (Provider, error) {
	apiURL := strings.TrimSpace(os.Getenv("RESEARCH_API_URL"))
	if apiURL == "" {
		apiURL = strings.TrimSpace(os.Getenv("SEARCH_API_URL"))
	}
	apiKey := strings.TrimSpace(os.Getenv("RESEARCH_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("SEARCH_API_KEY"))
	}

	switch strings.ToLower(name) {
	case "", "none", "off", "disabled":
		return nil, nil
	case "open-web":
		return OpenWeb(logger), nil
	case "search-api":
		if apiURL == "" {
			return nil, errors.New("RESEARCH_PROVIDER includes search-api but RESEARCH_API_URL is unset")
		}
		return NewSearchAPI(apiURL, apiKey, logger), nil
	}
	return nil, fmt.Errorf("unrecognised research backend %q", name)
}
