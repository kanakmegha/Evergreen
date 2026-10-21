package research

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// GBIFBaseURL is the GBIF API endpoint. It is a variable so a test can point
// it at a stub server.
var GBIFBaseURL = "https://api.gbif.org/v1"

// GBIFPageURL formats the public page for a GBIF taxon key.
func GBIFPageURL(key int64) string {
	return fmt.Sprintf("https://www.gbif.org/species/%d", key)
}

// GBIF is a research backend over the Global Biodiversity Information
// Facility's species API.
//
// Its job in the pipeline is step 6 — verify botanical identity — and nothing
// else. GBIF resolves a claimed binomial against the GBIF backbone taxonomy
// and reports whether that name is the accepted one and which family it sits
// in. That is a question an encyclopedia cannot answer reliably, and it is the
// single most important thing to check before Everroot shows a visitor a
// scientific name.
//
// What it does not do, and what Everroot therefore never claims on its
// authority: it does not say whether a species is native to a visitor's region,
// nor whether it is suitable to plant there. GBIF's occurrence data records
// where a species has been recorded, which includes cultivated and introduced
// specimens, so an occurrence count is not evidence of nativeness. Reporting
// one as such would be exactly the kind of confident-but-wrong claim this
// pipeline exists to prevent.
type GBIF struct {
	*baseBackend
}

// NewGBIF builds a GBIF backend. A nil logger is replaced with one that
// discards.
func NewGBIF(logger Logger) *GBIF {
	if logger == nil {
		logger = nopLogger{}
	}
	return &GBIF{&baseBackend{
		name:    "gbif",
		baseURL: GBIFBaseURL,
		logger:  logger,
		timeout: defaultTimeout,
	}}
}

// gbifSearch is the species search envelope. Only the fields Everroot reads
// are decoded.
type gbifSearch struct {
	Results []gbifTaxon `json:"results"`
}

type gbifTaxon struct {
	Key             int64  `json:"key"`
	ScientificName  string `json:"scientificName"`
	CanonicalName   string `json:"canonicalName"`
	Authorship      string `json:"authorship"`
	Rank            string `json:"rank"`
	TaxonomicStatus string `json:"taxonomicStatus"`
	Family          string `json:"family"`
	Genus           string `json:"genus"`
	Kingdom         string `json:"kingdom"`
	DatasetTitle    string `json:"datasetTitle"`
}

// Identity is what a GBIF lookup establishes about one claimed name.
type Identity struct {
	// Requested is the name Everroot asked about, verbatim.
	Requested string `json:"requested"`
	// ScientificName is GBIF's accepted name for the best match, empty when
	// there was no match.
	ScientificName string `json:"scientificName,omitempty"`
	// Family is the accepted family, empty when unknown.
	Family string `json:"family,omitempty"`
	// Status is GBIF's taxonomic status for the match ("ACCEPTED",
	// "SYNONYM", ...). It is empty when there was no match.
	Status string `json:"status,omitempty"`
	// Matched reports whether GBIF found a species-rank record at all.
	Matched bool `json:"matched"`
	// Source is the GBIF page for the matched taxon, empty when unmatched.
	Source string `json:"source,omitempty"`
}

// VerifyIdentity resolves a claimed scientific name against the GBIF backbone
// taxonomy.
//
// A lookup that fails at the transport level reports an error; a lookup that
// succeeds and finds nothing returns a zero Identity with Matched false. The
// difference matters: "GBIF has no record of this name" is a finding about the
// name, while "GBIF could not be reached" is a finding about our connection,
// and presenting the second as the first would tell a visitor a real species
// does not exist.
func (g *GBIF) VerifyIdentity(ctx context.Context, name string) (Identity, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Identity{}, nil
	}

	params := url.Values{}
	params.Set("q", name)
	// Restrict to a species-rank, currently accepted record so a match on a
	// subspecies or a superseded synonym is not mistaken for the accepted name.
	params.Set("rank", "SPECIES")
	params.Set("status", "ACCEPTED")
	params.Set("limit", "3")

	target, err := url.Parse(g.baseURL + "/species/search?" + params.Encode())
	if err != nil {
		return Identity{}, fmt.Errorf("%w: build query", ErrBadResponse)
	}

	body, err := g.doer().GetJSON(ctx, target)
	if err != nil {
		return Identity{}, err
	}

	var parsed gbifSearch
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Identity{}, fmt.Errorf("%w: decode species response", ErrBadResponse)
	}

	identity := Identity{Requested: name}
	for _, taxon := range parsed.Results {
		// Prefer an exact match on the canonical name. A "close enough" first
		// hit would let a misspelled binomial be reported as verified.
		canonical := strings.TrimSpace(taxon.CanonicalName)
		if canonical == "" {
			canonical = strings.TrimSpace(taxon.ScientificName)
		}
		if !strings.EqualFold(canonical, name) {
			continue
		}
		identity.Matched = true
		identity.ScientificName = strings.TrimSpace(taxon.ScientificName)
		identity.Family = strings.TrimSpace(taxon.Family)
		identity.Status = strings.TrimSpace(taxon.TaxonomicStatus)
		identity.Source = GBIFPageURL(taxon.Key)
		break
	}
	return identity, nil
}

// Search implements Provider.
//
// GBIF answers a botanical identity question rather than a free-text search,
// so a query with no species carries nothing to look up. Reporting
// ErrNoSources rather than an empty list keeps that distinct from a species
// that genuinely has no record.
func (g *GBIF) Search(ctx context.Context, q Query, limit int) ([]Source, error) {
	if strings.TrimSpace(q.Species) == "" {
		return nil, ErrNoSources
	}
	identity, err := g.VerifyIdentity(ctx, q.Species)
	if err != nil {
		return nil, err
	}
	if !identity.Matched {
		return nil, ErrNoSources
	}

	snippet := "GBIF backbone taxonomy records this name as " + identity.ScientificName
	if identity.Status != "" {
		snippet += " (status: " + identity.Status + ")"
	}
	if identity.Family != "" {
		snippet += ", in the family " + identity.Family + "."
	} else {
		snippet += "."
	}

	return []Source{{
		Title:       identity.ScientificName + " — GBIF Backbone Taxonomy",
		URL:         identity.Source,
		Publisher:   "GBIF — Global Biodiversity Information Facility",
		Kind:        KindBiodiversity,
		Snippet:     cleanText(snippet, maxSnippetChars),
		RetrievedAt: time.Now().UTC(),
	}}, nil
}

// OpenWeb is the key-free default: an encyclopedia backend for narrative
// context and a taxonomy backend for botanical identity, fanned out
// concurrently.
func OpenWeb(logger Logger) Provider {
	return Multi{
		Providers: []Provider{
			NewWikipedia(logger),
			NewGBIF(logger),
		},
	}
}
