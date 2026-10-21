package httpapi

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// The local reference list and the database seed are two copies of one
// vocabulary. Nothing forces them to agree, so this test does: it reads the
// migration that fills the `nakshatras` table and requires the served list to
// match it name for name and number for number. A spelling that drifts would
// quietly split a visitor's choice from the records it was filed under.
func TestNakshatraReferenceMatchesTheMigration(t *testing.T) {
	const migration = "../../db/migrations/0002_nakshatra_reference.sql"
	raw, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("read %s: %v", migration, err)
	}

	pair := regexp.MustCompile(`\('([^']+)',(\d+)\)`)
	matches := pair.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatalf("%s contains no (name, ordinal) rows", migration)
	}

	if len(nakshatraReference) != len(matches) {
		t.Fatalf("reference list has %d entries, migration seeds %d", len(nakshatraReference), len(matches))
	}
	for i, m := range matches {
		ordinal, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("migration ordinal %q: %v", m[2], err)
		}
		got := nakshatraReference[i]
		if got.Name != m[1] {
			t.Errorf("position %d: reference %q, migration %q", i, got.Name, m[1])
		}
		if got.Ordinal != ordinal {
			t.Errorf("position %d: reference ordinal %d, migration %d", i, got.Ordinal, ordinal)
		}
	}
}

// The vocabulary is fixed, and numbered from 1. A zero or a repeat would show
// up as a broken badge in the interface and as an ambiguous choice at the API.
func TestNakshatraReferenceIsCompleteAndUniquelyNumbered(t *testing.T) {
	if len(nakshatraReference) != 27 {
		t.Fatalf("got %d nakshatras, want the canonical 27", len(nakshatraReference))
	}
	seenName := map[string]bool{}
	seenOrdinal := map[int]bool{}
	for _, n := range nakshatraReference {
		if n.Name == "" {
			t.Error("a nakshatra has no name")
		}
		if n.Ordinal < 1 || n.Ordinal > 27 {
			t.Errorf("%q has ordinal %d, outside 1..27", n.Name, n.Ordinal)
		}
		if seenName[n.Name] {
			t.Errorf("duplicate name %q", n.Name)
		}
		if seenOrdinal[n.Ordinal] {
			t.Errorf("duplicate ordinal %d", n.Ordinal)
		}
		seenName[n.Name] = true
		seenOrdinal[n.Ordinal] = true
	}
}

// The tradition the vocabulary belongs to is the slug the frontend compares
// against and the one the reviewed mappings are filed under. All three have to
// be the same string or a choice made in the interface can never match a
// record.
func TestTraditionReferenceUsesTheAgreedSlug(t *testing.T) {
	if len(traditionReference) != 1 {
		t.Fatalf("got %d traditions, want the one the catalogue seeds", len(traditionReference))
	}
	got := traditionReference[0]
	if got.Slug != nakshatraTraditionSlug {
		t.Errorf("slug %q, want %q", got.Slug, nakshatraTraditionSlug)
	}
	if got.Name == "" {
		t.Error("the tradition has no display name")
	}
	if _, ok := regionReference[got.Slug]; !ok {
		t.Errorf("no regions recorded for the supported tradition %q", got.Slug)
	}
}

// regionsFor hands back a copy. A caller appending to the slice it returns must
// not be able to alter what the next caller is offered.
func TestRegionsForReturnsACopy(t *testing.T) {
	first := regionsFor(nakshatraTraditionSlug)
	if len(first) == 0 {
		t.Fatal("no regions for the supported tradition")
	}
	first[0] = "mutated by the caller"

	second := regionsFor(nakshatraTraditionSlug)
	if second[0] == "mutated by the caller" {
		t.Error("regionsFor handed out the reference slice itself; one caller changed the next one's options")
	}
}

// An unrecognised tradition yields an empty list rather than nil, so the JSON
// body is [] and a frontend iterating it does not have to special-case null.
func TestRegionsForUnknownTraditionIsEmptyNotNull(t *testing.T) {
	got := regionsFor("not-a-tradition")
	if got == nil {
		t.Fatal("regionsFor returned nil; it must marshal as []")
	}
	if len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

/* --------------------------------------------------------------- naming systems */

// A naming system is only offered if Everroot can actually serve names in it.
// A system listed without names would send a visitor to a step that silently
// falls back to another language, which is the thing this feature exists to
// avoid.
func TestEveryOfferedNamingSystemServesAllTwentySeven(t *testing.T) {
	for _, system := range namingReference {
		names, supported := nakshatraNamesFor(system.Code)
		if !supported {
			t.Errorf("naming system %q is offered but not recognised", system.Code)
		}
		if len(names) != 27 {
			t.Errorf("naming system %q served %d names, want 27", system.Code, len(names))
		}
		for _, entry := range names {
			if entry.Name == "" {
				t.Errorf("naming system %q has no name at ordinal %d", system.Code, entry.Ordinal)
			}
		}
	}
}

// The Tamil names are paired to the nakshatras by position, so the one thing
// that must never go wrong is a name sitting against the wrong number. Every
// entry has to keep its ordinal and carry the romanised name it stands for, so
// a visitor's choice still reaches the same record the model is asked about.
func TestTamilNamesPairWithTheCanonicalSequence(t *testing.T) {
	names, supported := nakshatraNamesFor("ta")
	if !supported {
		t.Fatal("Tamil is offered but not recognised")
	}
	if len(names) != len(nakshatraReference) {
		t.Fatalf("got %d Tamil names, want %d", len(names), len(nakshatraReference))
	}
	for i, entry := range names {
		canonical := nakshatraReference[i]
		if entry.Ordinal != canonical.Ordinal {
			t.Errorf("position %d carries ordinal %d, want %d — a name against the wrong number",
				i, entry.Ordinal, canonical.Ordinal)
		}
		if entry.Canonical != canonical.Name {
			t.Errorf("position %d: canonical %q, want %q", i, entry.Canonical, canonical.Name)
		}
	}
}

// The list has to be Tamil, not the romanised list wearing a Tamil label. The
// script is the check a reader can make without reading Tamil: a name in the
// Tamil naming system must be written in Tamil script.
func TestTamilNamesAreActuallyInTamilScript(t *testing.T) {
	if len(tamilNakshatraNames) != 27 {
		t.Fatalf("got %d Tamil names, want the canonical 27", len(tamilNakshatraNames))
	}
	for i, name := range tamilNakshatraNames {
		if name == "" {
			t.Fatalf("position %d has no Tamil name", i)
		}
		if !containsTamilScript(name) {
			t.Errorf("%q is not written in Tamil script; relabelling the romanised list would not count",
				name)
		}
	}
	// Two systems offering the identical spelling means one of them is a copy
	// of the other rather than a naming system of its own.
	canonical, _ := nakshatraNamesFor(canonicalNamingCode)
	tamil, _ := nakshatraNamesFor("ta")
	same := 0
	for i := range canonical {
		if canonical[i].Name == tamil[i].Name {
			same++
		}
	}
	if same > 0 {
		t.Errorf("%d of 27 Tamil names are identical to the romanised ones", same)
	}
}

// containsTamilScript reports whether a string carries a Tamil-block rune.
func containsTamilScript(s string) bool {
	for _, r := range s {
		if r >= 0x0B80 && r <= 0x0BFF {
			return true
		}
	}
	return false
}

// A naming system Everroot does not hold is not an error — the twenty-seven are
// the same sequence under every name — but the caller has to be told, so it can
// say the names shown are not the ones that were asked for rather than
// presenting romanised spellings as a match.
func TestUnknownNamingSystemFallsBackAndSaysSo(t *testing.T) {
	names, supported := nakshatraNamesFor("gr")
	if supported {
		t.Error("an unoffered naming system reported as supported")
	}
	if len(names) != 27 {
		t.Fatalf("got %d names, want all 27 regardless", len(names))
	}
	canonical, _ := nakshatraNamesFor(canonicalNamingCode)
	for i, entry := range names {
		if entry.Name != canonical[i].Name {
			t.Errorf("position %d: %q, want the canonical %q", i, entry.Name, canonical[i].Name)
		}
	}
}

// The unqualified list and the explicitly-canonical one are the same list. If
// they ever differ, a visitor who chose nothing and a visitor who chose
// "Sanskrit" would be shown different spellings of the same nakshatra.
func TestCanonicalNamingMatchesTheUnqualifiedList(t *testing.T) {
	plain, _ := nakshatraNamesFor(canonicalNamingCode)
	reference := nakshatraReference
	if len(plain) != len(reference) {
		t.Fatalf("canonical served %d, the unqualified list has %d", len(plain), len(reference))
	}
	for i, entry := range plain {
		if entry.Name != reference[i].Name {
			t.Errorf("position %d: %q, want %q", i, entry.Name, reference[i].Name)
		}
		if entry.Canonical != reference[i].Name {
			t.Errorf("position %d canonical field %q, want %q", i, entry.Canonical, reference[i].Name)
		}
	}
}

// Every offered naming system is listed, and nothing is listed that has no
// names behind it — the two directions of the same promise.
func TestNamingSystemsListAgreesWithTheNamesServed(t *testing.T) {
	offered := map[string]bool{}
	for _, system := range namingSystems() {
		if system.Code == "" || system.Name == "" || system.Endonym == "" || system.Script == "" {
			t.Errorf("naming system %+v is missing a field the interface renders", system)
		}
		if offered[system.Code] {
			t.Errorf("duplicate naming system code %q", system.Code)
		}
		offered[system.Code] = true
	}
	for code := range namesByNamingCode {
		if !offered[code] {
			t.Errorf("names exist for %q but it is never offered", code)
		}
	}
}

// The served list is a copy. A caller appending to it must not be able to alter
// what the next caller is offered.
func TestNamingSystemsReturnsACopy(t *testing.T) {
	first := namingSystems()
	first[0].Code = "mutated"
	second := namingSystems()
	if second[0].Code == "mutated" {
		t.Error("namingSystems handed out the reference slice; one caller changed the next one's options")
	}
}
