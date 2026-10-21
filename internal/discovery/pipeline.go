// Package discovery runs one Everroot discovery from start to finish.
//
// It is the layer that decides the shape of an answer, and it deliberately
// holds no HTTP and no SQL. The pipeline is:
//
//	interpret  the visitor's answers, reduced to what the question needs
//	research   retrieve sources dynamically; a failure degrades, never blocks
//	generate  one model call, grounded in what was retrieved
//	verify     check each candidate's binomial against a taxonomic record
//	validate  repair what the server owns: citations, confidence, identity
//
// Two rules hold across all five.
//
// A provider failure is an error, never an empty answer. "Everroot has nothing
// for you" and "Everroot could not reach its model" look identical as an empty
// array, and conflating them is the single most damaging thing this pipeline
// could do. An inference failure returns an error the HTTP layer turns into an
// actionable message.
//
// A research failure is a labelled downgrade, not a block. When sources cannot
// be retrieved the visitor still gets candidates, marked as provisional and
// described as un-researched. A database outage is not consulted at all: no
// part of this pipeline reads PostgreSQL, so discovery works with the database
// empty or unreachable.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"everroot/internal/ai"
	"everroot/internal/research"
)

// Suggester is the slice of the AI client this package needs. A nil Suggester
// means the model is switched off, which is reported as unconfigured.
type Suggester interface {
	Discover(ctx context.Context, in ai.DiscoverInput) (ai.DiscoverResult, error)
}

// Verifier resolves a claimed scientific name against a taxonomic record.
type Verifier interface {
	VerifyIdentity(ctx context.Context, name string) (research.Identity, error)
}

// Status values for one discovery. They are the same strings the prompt
// receives, so what the model was told and what the visitor is shown cannot
// disagree.
const (
	StatusResearched  = ai.ResearchStatusResearched
	StatusPartial     = ai.ResearchStatusPartial
	StatusUnavailable = ai.ResearchStatusUnavailable
)

// Request is one visitor's answers.
type Request struct {
	Intention   string
	Description string
	Tradition   string
	Region      string
	Nakshatra   string
	Constraints []string
	Limit       int
}

// Outcome is everything one discovery produced, including what went wrong.
type Outcome struct {
	// Result is the generated answer. It is meaningful only when Err is nil.
	Result ai.DiscoverResult
	// Sources are the retrieved findings, with server-assigned IDs matching
	// the citations inside Result.
	Sources []research.Source
	// Status is researched, partial or unavailable.
	Status string
	// Err is a provider or inference failure. A research failure is NOT an
	// error: it is Status.
	Err error
}

// Pipeline runs discoveries. Its fields are all optional except Suggester: a
// Pipeline with no research provider performs no research and says so.
type Pipeline struct {
	Suggester Suggester
	Research  research.Provider
	Verifier  Verifier
	Log       *slog.Logger

	// ResearchTimeout bounds the research phase on its own, so a slow search
	// host cannot consume the budget the model call needs.
	ResearchTimeout time.Duration
	// SourceLimit is how many sources to retrieve and to show the model.
	SourceLimit int
}

// defaults, kept small so one discovery stays inside a request path.
const (
	defaultResearchTimeout = 8 * time.Second
	defaultSourceLimit     = 8
	// verifyTimeout bounds the whole identity-verification fan-out.
	verifyTimeout = 6 * time.Second
	// verifyConcurrency bounds how many taxonomic lookups run at once.
	verifyConcurrency = 4
)

// Run executes one discovery.
//
// The order matters and is not incidental. Research runs before the model call
// so the model is grounded rather than recalling; verification runs after, so
// the model's binomials are checked rather than trusted. Nothing in Run reads
// the database.
func (p *Pipeline) Run(ctx context.Context, req Request) Outcome {
	if p.Suggester == nil {
		return Outcome{Status: StatusUnavailable, Err: ai.ErrNotConfigured}
	}

	sources, status := p.research(ctx, req)

	input := ai.DiscoverInput{
		Intention:       req.Intention,
		Description:     req.Description,
		Tradition:       req.Tradition,
		Region:          req.Region,
		Nakshatra:       req.Nakshatra,
		Constraints:     req.Constraints,
		Count:           req.Limit,
		Sources:         toPromptSources(sources),
		ResearchStatus:  status,
		ResearchSummary: ai.DiscoverResearchSummary(status, len(sources)),
	}

	result, err := p.Suggester.Discover(ctx, input)
	if err != nil {
		// A model failure is an error. It is never reported as a discovery
		// that happened to contain nothing.
		return Outcome{Status: status, Err: err}
	}

	// Research facts belong to the pipeline, not to the model. Overwriting
	// them here means a reply that misreported its own research status can
	// never reach a visitor: what is shown is what actually happened.
	result.ResearchStatus = status
	result.ResearchSummary = ai.DiscoverResearchSummary(status, len(sources))

	p.verifyIdentities(ctx, result.Candidates)

	return Outcome{
		Result:  result,
		Sources: sources,
		Status:  status,
	}
}

// research retrieves sources for one request and reports how it went.
//
// Every failure path here returns a status, never an error. The visitor asked
// about trees, not about search backends, and a discovery that could not be
// researched is still an answer — as long as it is labelled, which is exactly
// what the status does.
func (p *Pipeline) research(ctx context.Context, req Request) ([]research.Source, string) {
	if p.Research == nil {
		return nil, StatusUnavailable
	}

	terms := ai.DiscoverResearchTerms(req.toAIInput())
	if len(terms) == 0 {
		// Nothing to search for. That is not a failure; it is a request that
		// gave the pipeline no words, which the follow-up question exists for.
		return nil, StatusUnavailable
	}

	limit := p.SourceLimit
	if limit <= 0 {
		limit = defaultSourceLimit
	}
	timeout := p.ResearchTimeout
	if timeout <= 0 {
		timeout = defaultResearchTimeout
	}

	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sources, err := p.Research.Search(rctx, research.Query{
		Terms:   terms,
		Species: "",
		Region:  req.Region,
	}, limit)

	// ErrPartial travels WITH findings: some backends answered and some did
	// not. It is handled first, because it is not a failure of the lookup —
	// the reply is grounded, just not as fully as it would have been.
	if err != nil && errors.Is(err, research.ErrPartial) {
		if len(sources) == 0 {
			return nil, StatusUnavailable
		}
		return sources, StatusPartial
	}

	if err != nil {
		// ErrNoSources means the backends answered and found nothing, which is
		// a fact about the world rather than an outage. It is still not
		// "researched", because there is nothing to have grounded the answer
		// in, but it is not a failure to report either.
		if !errors.Is(err, research.ErrNoSources) {
			diag := research.Diagnose(err)
			p.log().Warn("research failed; continuing with a provisional answer",
				"category", string(diag.Category),
				"retryable", diag.Retryable,
				"detail", diag.Detail,
			)
		}
		return nil, StatusUnavailable
	}

	if len(sources) == 0 {
		return nil, StatusUnavailable
	}
	return sources, StatusResearched
}

// toAIInput is the request reduced to what the prompt builder needs. It exists
// so research terms and the prompt draw on exactly the same reduction and can
// never drift apart.
func (r Request) toAIInput() ai.DiscoverInput {
	return ai.DiscoverInput{
		Intention:   r.Intention,
		Description: r.Description,
		Tradition:   r.Tradition,
		Region:      r.Region,
		Nakshatra:   r.Nakshatra,
		Constraints: r.Constraints,
		Count:       r.Limit,
	}
}

// toPromptSources maps retrieved findings onto the prompt's shape and assigns
// the IDs the model will cite.
//
// The IDs are assigned here, in a fixed order, so the same findings always
// produce the same IDs. The model is shown these and returns them verbatim; an
// ID it invented is rejected when the reply is validated.
func toPromptSources(sources []research.Source) []ai.DiscoverSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]ai.DiscoverSource, 0, len(sources))
	for i, s := range sources {
		sources[i].ID = fmt.Sprintf("s%d", i+1)
		out = append(out, ai.DiscoverSource{
			ID:        sources[i].ID,
			Title:     s.Title,
			Publisher: s.Publisher,
			Snippet:   s.Snippet,
		})
	}
	return out
}

// verifyIdentities checks each candidate's binomial against a taxonomic
// record and writes the verdict into the candidate.
//
// The verdict is never the model's word for it. When no verifier is
// configured, or a candidate carries no binomial, the status is "not_attempted"
// rather than "confirmed" — an unverified name must never be able to read as
// a verified one.
func (p *Pipeline) verifyIdentities(ctx context.Context, candidates []ai.Candidate) {
	// Every candidate must end with a status, and "not_attempted" is the only
	// honest one when there was no check. A blank status must never be able to
	// render as though a name had been confirmed.
	if p.Verifier == nil {
		for i := range candidates {
			if candidates[i].Identity.Status == "" {
				candidates[i].Identity = ai.Identity{Status: ai.IdentityNotAttempted}
			}
		}
		return
	}
	if len(candidates) == 0 {
		return
	}

	vctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	sem := make(chan struct{}, verifyConcurrency)
	var wg sync.WaitGroup
	for i := range candidates {
		name := strings.TrimSpace(candidates[i].ScientificName)
		if name == "" {
			// Nothing to check. Stating that plainly beats leaving the field
			// to be read as an unremarked success.
			candidates[i].Identity = ai.Identity{Status: ai.IdentityNotAttempted}
			continue
		}
		wg.Add(1)
		go func(idx int, binomial string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			identity, err := p.Verifier.VerifyIdentity(vctx, binomial)
			if err != nil {
				// A lookup that failed is not a lookup that found nothing.
				// "unverified" says we do not know; "not_found" would assert
				// the name is wrong, and nothing here established that.
				p.log().Warn("botanical identity lookup failed",
					"category", string(research.Diagnose(err).Category))
				candidates[idx].Identity = ai.Identity{Status: ai.IdentityUnverified}
				return
			}
			if !identity.Matched {
				candidates[idx].Identity = ai.Identity{Status: ai.IdentityNotFound}
				return
			}
			candidates[idx].Identity = ai.Identity{
				Status:       ai.IdentityConfirmed,
				AcceptedName: identity.ScientificName,
				Family:       identity.Family,
				Source:       identity.Source,
			}
		}(i, name)
	}
	wg.Wait()
}

func (p *Pipeline) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.Default()
}

// SourcesFor returns every source a candidate cited, in citation order.
//
// It is the mapping the interface uses to render a card's citation list from
// the IDs the model returned. A citation that resolves to nothing is skipped
// rather than rendered as a broken entry, which is the state validation has
// already guaranteed cannot happen.
func SourcesFor(sources []research.Source, ids []string) []research.Source {
	if len(ids) == 0 {
		return nil
	}
	byID := make(map[string]research.Source, len(sources))
	for _, s := range sources {
		byID[s.ID] = s
	}
	out := make([]research.Source, 0, len(ids))
	for _, id := range ids {
		if s, ok := byID[id]; ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// DistinctSources flattens every citation across every candidate and returns
// the unique sources, in first-cited order. The results page renders this as
// its "sources consulted" list.
func DistinctSources(sources []research.Source, candidates []ai.Candidate) []research.Source {
	seen := map[string]bool{}
	var ids []string
	for _, c := range candidates {
		for _, id := range c.SourceIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	out := SourcesFor(sources, ids)
	// A stable, deterministic order keeps the rendered list from shifting
	// between reads of the same stored result.
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
