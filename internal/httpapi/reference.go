package httpapi

// The input vocabulary Everroot offers a visitor — the 27 nakshatras, the
// traditions to attach them to, and the regions those traditions are recorded
// in — is defined here rather than read from PostgreSQL.
//
// It is reference data, not reviewed evidence. Nothing on this file asserts
// that a particular tree is associated with a particular nakshatra; it only
// names the options a visitor may choose from, which is exactly what the
// migrations seed into the `nakshatras`, `traditions` and `tradition_locales`
// tables. Serving it from here means discovery never depends on a database
// being reachable, migrated or seeded: an empty catalogue and a full one offer
// the same choices.
//
// The reviewed catalogue keeps its own job, and is unaffected. `GET
// /api/v1/trees` still reads the botanical records it can only get from the
// database, and `POST /api/v1/discoveries` still answers with reviewed
// mappings — degrading to "no verified mapping" rather than inventing one. This
// file removes a prerequisite; it does not remove the review.

// nakshatraTraditionSlug is the tradition the nakshatra vocabulary belongs to.
// It is the same slug migration 0004 inserts, and the same one the frontend
// compares against (see NAKSHATRA_TRADITION in discovery.ts).
const nakshatraTraditionSlug = "nakshatra-tree-association"

// traditionOption is one selectable tradition. The shape is the one the
// frontend's Tradition interface already declares.
type traditionOption struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// nakshatraOption is one selectable nakshatra, numbered from 1. The shape is
// the one the frontend's Nakshatra interface already declares.
type nakshatraOption struct {
	Name    string `json:"name"`
	Ordinal int    `json:"ordinal"`
}

// traditionReference is the supported-traditions list. Migration 0004 seeds
// exactly this one row, so a populated database and this list agree.
var traditionReference = []traditionOption{
	{Slug: nakshatraTraditionSlug, Name: "Nakshatra tree association"},
}

// nakshatraReference is the canonical 27 nakshatras with their traditional
// ordering, mirroring migration 0002 name for name and number for number.
//
// Spellings are the migration's, deliberately: a visitor who picks "Mula" here
// should send the same string the reviewed records were filed under, and a
// second spelling would quietly split the two. The names are an input
// vocabulary and assert no association by themselves.
var nakshatraReference = []nakshatraOption{
	{Name: "Ashwini", Ordinal: 1},
	{Name: "Bharani", Ordinal: 2},
	{Name: "Krittika", Ordinal: 3},
	{Name: "Rohini", Ordinal: 4},
	{Name: "Mrigashira", Ordinal: 5},
	{Name: "Ardra", Ordinal: 6},
	{Name: "Punarvasu", Ordinal: 7},
	{Name: "Pushya", Ordinal: 8},
	{Name: "Ashlesha", Ordinal: 9},
	{Name: "Magha", Ordinal: 10},
	{Name: "Purva Phalguni", Ordinal: 11},
	{Name: "Uttara Phalguni", Ordinal: 12},
	{Name: "Hasta", Ordinal: 13},
	{Name: "Chitra", Ordinal: 14},
	{Name: "Swati", Ordinal: 15},
	{Name: "Vishakha", Ordinal: 16},
	{Name: "Anuradha", Ordinal: 17},
	{Name: "Jyeshtha", Ordinal: 18},
	{Name: "Mula", Ordinal: 19},
	{Name: "Purva Ashadha", Ordinal: 20},
	{Name: "Uttara Ashadha", Ordinal: 21},
	{Name: "Shravana", Ordinal: 22},
	{Name: "Dhanishta", Ordinal: 23},
	{Name: "Shatabhisha", Ordinal: 24},
	{Name: "Purva Bhadrapada", Ordinal: 25},
	{Name: "Uttara Bhadrapada", Ordinal: 26},
	{Name: "Revati", Ordinal: 27},
}

// regionReference lists the regions each tradition is recorded in. Migration
// 0004 inserts these two locales for the nakshatra tradition.
//
// A region narrows which reviewed records are shown; it is never a
// prerequisite for a recommendation. An unrecognised tradition returns no
// regions rather than an error, because the field it feeds is optional and a
// missing list must not stop a visitor.
var regionReference = map[string][]string{
	nakshatraTraditionSlug: {"Tamil Nadu", "North India"},
}

// regionsFor returns the recorded regions for a tradition slug, or an empty
// list when there are none. The result is never nil, so it marshals as [] and
// not as null.
func regionsFor(slug string) []string {
	regions := regionReference[slug]
	if regions == nil {
		return []string{}
	}
	// Copy, so a caller cannot mutate the reference data itself.
	out := make([]string, len(regions))
	copy(out, regions)
	return out
}

/* --------------------------------------------------------------- naming systems */

// A visitor who reaches for a nakshatra usually knows the name in their own
// language, not in the romanised Sanskrit the reference list is filed under.
// Offering only the romanised form quietly makes one tradition's spelling the
// default for everyone, so each naming system Everroot holds is listed and the
// names are served in it.
//
// This is vocabulary again, and only vocabulary. A Tamil name for the 19th
// nakshatra says nothing about which tree belongs to it — the association is
// still the model's answer, and the publication gate still reports that no
// reviewer has confirmed one.
type namingSystem struct {
	// Code is the stable identifier sent back to the API. "canonical" is the
	// romanised Sanskrit list the reviewed records are filed under.
	Code string `json:"code"`
	// Name is how the system is described in English.
	Name string `json:"name"`
	// Endonym is how speakers of the language name it themselves, so the choice
	// can be recognised without reading an English label.
	Endonym string `json:"endonym"`
	// Script names the writing system the served names use.
	Script string `json:"script"`
}

// canonicalNamingCode is the naming system whose spellings match the reviewed
// records, so it is what an unqualified request returns.
const canonicalNamingCode = "canonical"

// namingReference is the set of naming systems Everroot holds names for.
//
// Tamil is offered because the seeded catalogue records a Tamil reference
// context for this tradition — migration 0004 puts a `ta-IN` row in
// tradition_locales, labelled "Tamil Nadu" / "Tamil reference context" — so a
// visitor who reaches the nakshatra step from there should be offered names
// they recognise rather than transliterated ones.
//
// The names themselves are not in the database: migration 0001 creates
// `nakshatra_names` and nothing has ever seeded it. They are local reference
// vocabulary of the same kind as the canonical list, paired to it by ordinal,
// and like it they assert nothing about any tree. Hindi has a locale row too
// and is deliberately absent here — Everroot has no Hindi names to serve, and
// offering a naming system with nothing behind it would be worse than leaving
// it off.
var namingReference = []namingSystem{
	{Code: canonicalNamingCode, Name: "Sanskrit, in English script", Endonym: "Sanskrit", Script: "Latin"},
	{Code: "ta", Name: "Tamil", Endonym: "தமிழ்", Script: "Tamil"},
}

// tamilNakshatraNames is the same twenty-seven, in the same order, named in
// Tamil — the standard Tamil nomenclature, written in Tamil script, not the
// romanised list transliterated back. The pairing is by ordinal, which is why
// the ordering is the part that must never drift: a name filed against the
// wrong number would put a visitor on a different nakshatra than the one they
// chose.
var tamilNakshatraNames = []string{
	"அசுவினி", "பரணி", "கார்த்திகை", "ரோகிணி", "மிருகசீரிஷம்", "திருவாதிரை",
	"புனர்பூசம்", "பூசம்", "ஆயில்யம்", "மகம்", "பூரம்", "உத்திரம்",
	"அஸ்தம்", "சித்திரை", "சுவாதி", "விசாகம்", "அனுஷம்", "கேது",
	"மூலம்", "பூராடம்", "உத்திராடம்", "திருவோணம்", "அவிட்டம்", "சதயம்",
	"பூரட்டாதி", "உத்திரட்டாதி", "ரேவதி",
}

// namesByNamingCode maps a naming system to its twenty-seven names, indexed by
// position rather than by name: the ordinal is the only thing tying a Tamil
// name to the nakshatra the romanised list calls "Mula". The canonical list is
// deliberately absent — it is the index itself, so there is no second copy to
// fall out of step with the first.
var namesByNamingCode = map[string][]string{
	"ta": tamilNakshatraNames,
}

// namedNakshatra is one nakshatra as a visitor following a chosen naming system
// should see it. Canonical is always present and is the romanised form the
// reviewed records and the model are addressed with, so the choice a visitor
// makes in their own script still reaches the same record.
type namedNakshatra struct {
	Name    string `json:"name"`
	Ordinal int    `json:"ordinal"`
	// Canonical is the romanised Sanskrit name for the same nakshatra. It is
	// the value sent on to the model, so a display name never has to double as
	// a lookup key.
	Canonical string `json:"canonical"`
}

// namingSystems returns the naming systems Everroot holds names for. The result
// is never nil, so it marshals as [] rather than null.
func namingSystems() []namingSystem {
	out := make([]namingSystem, len(namingReference))
	copy(out, namingReference)
	return out
}

// namingByCode finds a supported naming system.
func namingByCode(code string) (namingSystem, bool) {
	for _, system := range namingReference {
		if system.Code == code {
			return system, true
		}
	}
	return namingSystem{}, false
}

// nakshatraNamesFor serves the twenty-seven in the named naming system.
//
// An unrecognised code yields the canonical list rather than an error. A
// visitor who asked for a naming system Everroot does not hold still gets the
// twenty-seven — the sequence is the same in every one of them — and the false
// second return tells the caller to say so rather than quietly present
// romanised names as though they were the ones that were asked for.
func nakshatraNamesFor(code string) ([]namedNakshatra, bool) {
	// The canonical list is the index itself, so it needs no separate table.
	names := namesByNamingCode[code]

	out := make([]namedNakshatra, 0, len(nakshatraReference))
	for i, canonical := range nakshatraReference {
		name := canonical.Name
		if i < len(names) {
			name = names[i]
		}
		out = append(out, namedNakshatra{
			Name:      name,
			Ordinal:   canonical.Ordinal,
			Canonical: canonical.Name,
		})
	}
	_, supported := namingByCode(code)
	return out, supported
}
