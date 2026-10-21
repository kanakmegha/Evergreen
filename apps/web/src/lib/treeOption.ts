/* ---------------------------------------------------------------------------
   Normalised option view models.

   Two very different kinds of record have to sit in the same compact card: a
   reviewed cultural association (Track A) and a botanical reference record
   (Track B). They must never be dressed up as each other, so the differences
   that matter — what kind of claim this is, whether a cultural association was
   verified, and which gaps are honest to admit — are carried as explicit fields
   rather than inferred from styling.
--------------------------------------------------------------------------- */

import type {
  BotanicalCandidate,
  DiscoverySource,
  Recommendation,
  RecommendationSuggestion,
  TreeFact,
} from './discovery';
import { validURL } from './discovery';

/** A small label on the card: evidence type, review state, or membership. */
export type OptionTag = {
  text: string;
  tone?: 'plain' | 'neutral' | 'pending' | 'unknown';
};

export type OptionFact = { label: string; value: string };

export type OptionSource = {
  title: string;
  url: string;
  meta: string;
  tags: string[];
  attribution: string;
};

/** The shape one option card renders, whatever record it came from. */
export type TreeOptionView = {
  key: string;
  /** Common name when the records carry one, otherwise the scientific name. */
  title: string;
  /** True when the title is a botanical name, so it is styled as one. */
  titleIsScientific: boolean;
  /** A one-character mark. Decorative, never an illustration of the tree. */
  mark: string;
  scientificName: string;
  authorship: string;
  tags: OptionTag[];
  /** The honest reason this option is shown. Kept to one or two sentences. */
  whyIncluded: string;
  /** One sentence stating whether a cultural association was verified. */
  culturalNote: string;
  culturalVerified: boolean;
  facts: OptionFact[];
  otherNames: string[];
  gaps: string[];
  sources: OptionSource[];
  /** Present for botanical reference records. Reviewed records carry none. */
  tree: TreeFact | null;
  /** Where the option can be opened on its own. Empty when there is nowhere to go. */
  referenceHref: string;
  referenceLabel: string;
};

const REVIEWED = new Set(['verified', 'approved', 'published', 'reviewed']);

/**
 * The selection rule, stated plainly. The botanical reference is ordered by
 * family then accepted name and knows nothing about the visitor, so nothing on
 * it is a recommendation and nothing on it is matched to a Nakshatra. The
 * cultural point itself is made once at the top of the page and again in each
 * option's evidence panel, rather than on every card.
 */
export const REFERENCE_RULE =
  'A record from Everroot’s botanical reference, listed because it exists there — not because it matches your answers, and not a recommendation.';

/** Facts Everroot has no reviewed source for. Admitted, never filled in. */
export const REFERENCE_GAPS = [
  'Habitat and native range — no reviewed record',
  'Ecological role — no reviewed record',
  'Documented cultural or historical stories — no reviewed record',
];

/** What an AI suggestion is, stated on the card itself and never softened. */
export const SUGGESTION_RULE =
  'Chosen for you from your answers and general growing knowledge — written for your situation, not read from a reviewed record.';

/**
 * What a suggestion does not have. The absence of a source is the first thing
 * on the list because it is the one difference between this card and a record
 * Everroot could stand behind. The wording states the gap plainly without
 * arguing with the visitor about the choice they just made.
 */
export const SUGGESTION_GAPS = [
  'Source — none: written for you, not quoted from a reviewed record',
  'Cultural association — only a reviewed Everroot record can state one',
  'Habitat, native range and ecological role — no reviewed record',
];

export function titleCase(value: string): string {
  return value ? value.charAt(0).toUpperCase() + value.slice(1) : value;
}

/**
 * The display label for a species. Authorities list vernacular names
 * alphabetically, so the first entry is often a qualified form such as "common
 * mango". The shortest recorded name reads best and is never invented — every
 * name is still listed in full on the card — and the rule is deterministic.
 */
export function displayCommonName(names: string[]): string {
  if (!names.length) return '';
  return [...names].sort((a, b) => a.length - b.length || a.localeCompare(b))[0];
}

/**
 * Narrowing of the already-loaded reference. Matching is done in the browser, so
 * a search term never leaves the page and nothing has to be re-fetched.
 */
export function filterTrees(trees: TreeFact[], term: string): TreeFact[] {
  const needle = term.trim().toLowerCase();
  if (!needle) return trees;
  return trees.filter((tree) =>
    [
      tree.acceptedName,
      tree.family,
      tree.genus,
      tree.order,
      ...(tree.commonNames ?? []).map((entry) => entry.name),
    ]
      .join(' ')
      .toLowerCase()
      .includes(needle)
  );
}

export function humanise(value: string): string {
  return (value || 'unknown').replace(/[_-]+/g, ' ');
}

export function reviewTone(value: string): OptionTag['tone'] {
  const normalised = (value || '').toLowerCase();
  if (REVIEWED.has(normalised)) return 'plain';
  if (!normalised) return 'unknown';
  return 'pending';
}

/** Confidence may arrive as a 0–1 ratio or a 0–100 percentage. */
export function formatConfidence(value: number): string {
  if (!Number.isFinite(value)) return 'confidence not recorded';
  const ratio = value > 1 ? value / 100 : value;
  return `${Math.round(ratio * 100)}% confidence`;
}

export function candidateMeta(candidate: BotanicalCandidate): string {
  const parts = [humanise(candidate.certainty), formatConfidence(candidate.confidence)];
  if (!candidate.definitive) parts.push('not definitive');
  return parts.join(' · ');
}

/** Uncertainty is read off the record itself — never asserted on its behalf. */
export function uncertaintyNotes(rec: Recommendation): string[] {
  const notes: string[] = [];
  const candidates = rec.botanicalCandidates ?? [];
  const nonDefinitive = candidates.filter((c) => !c.definitive);
  if (candidates.length === 0) {
    notes.push('No botanical identification is on record for this association.');
  } else if (nonDefinitive.length > 0) {
    notes.push(
      `${nonDefinitive.length} of ${candidates.length} botanical identifications are not marked definitive.`
    );
  }
  if (!rec.sources?.length) {
    notes.push('No source is attached, so this record cannot be treated as verified.');
  } else {
    const unverified = (rec.sources ?? []).filter(
      (s) => !REVIEWED.has((s.verificationStatus || '').toLowerCase())
    );
    if (unverified.length > 0) {
      notes.push(`${unverified.length} of ${rec.sources.length} sources are not marked verified yet.`);
    }
  }
  if (!rec.region) {
    notes.push('This association is not tied to one region, and local practice may differ.');
  }
  if (notes.length === 0) {
    notes.push(
      'The sources for this record are marked verified. Community practice can still differ from what a text records.'
    );
  }
  return notes;
}

export function sourceTitle(source: DiscoverySource): string {
  return source.title || 'Untitled source';
}

export function sourceTags(source: DiscoverySource): string[] {
  const tags: string[] = [];
  if (source.verificationStatus) tags.push(humanise(source.verificationStatus));
  if (source.reuseStatus) tags.push(`reuse: ${humanise(source.reuseStatus)}`);
  return tags;
}

/**
 * One sentence about the cultural side, taken from the server's own publication
 * gate rather than guessed here.
 *
 * The unverified cases are stated as an absence of a record, never as a denial
 * of the tradition the visitor chose: they picked it in good faith, and the
 * sentence exists to explain what Everroot does not hold, not to argue.
 */
export function culturalNote(verified: boolean, state: string): string {
  if (verified) return 'Reviewed and published against the sources listed below.';
  switch (state) {
    case 'no_reviewed_match':
      return 'Everroot holds no reviewed record for this combination yet, so nothing cultural is claimed either way.';
    case 'not_queryable':
      return 'You did not pick a tradition or Nakshatra, so there was no cultural record to look up.';
    default:
      return 'Everroot has not verified a cultural association for this option.';
  }
}

/**
 * A botanical reference record. Everything shown is either an accepted
 * authority fact or an explicit statement that no reviewed record exists.
 */
export function referenceOption(
  tree: TreeFact,
  index: number,
  scope: string,
  inShortlist: boolean,
  associationState = ''
): TreeOptionView {
  const names = (tree.commonNames ?? []).map((entry) => entry.name);
  const display = displayCommonName(names);

  const facts: OptionFact[] = [];
  if (tree.family) facts.push({ label: 'Family', value: tree.family });
  if (tree.genus) facts.push({ label: 'Genus', value: tree.genus });
  if (tree.order) facts.push({ label: 'Order', value: tree.order });
  if (tree.rank) facts.push({ label: 'Rank', value: titleCase(humanise(tree.rank).toLowerCase()) });
  if (tree.protologue) facts.push({ label: 'First described', value: tree.protologue });

  const sources: OptionSource[] = [
    {
      title: `${tree.authority.dataset || 'Authority dataset'} taxon record`,
      url: validURL(tree.authority.url) ? tree.authority.url : '',
      meta: [
        tree.authority.doi ? `doi:${tree.authority.doi}` : '',
        tree.authority.taxonKey ? `taxon key ${tree.authority.taxonKey}` : '',
      ]
        .filter(Boolean)
        .join(' · '),
      tags: ['authority match', 'automated import check'],
      attribution: '',
    },
    // The authority source is already recorded upstream; do not list it twice.
    ...(tree.sources ?? [])
      .filter((source) => sourceTitle(source) !== tree.authority.dataset)
      .map((source) => ({
        title: sourceTitle(source),
        url: validURL(source.url) ? source.url : '',
        meta: [source.bibliographicReference, source.locator ? `locator: ${source.locator}` : '']
          .filter(Boolean)
          .join(' · '),
        tags: sourceTags(source),
        attribution: source.attributionRequirements
          ? `Attribution required: ${source.attributionRequirements}`
          : '',
      })),
  ];

  const tags: OptionTag[] = [{ text: 'Botanical record', tone: 'neutral' }];
  if (tree.taxonStatus) tags.push({ text: `${tree.taxonStatus.toLowerCase()} name`, tone: 'neutral' });
  if (!inShortlist) tags.push({ text: 'in your shortlist', tone: 'neutral' });

  return {
    key: `${scope}-${tree.speciesId}-${index}`,
    title: display ? titleCase(display) : tree.acceptedName,
    titleIsScientific: !display,
    mark: (tree.genus || tree.acceptedName || '?').charAt(0).toUpperCase(),
    scientificName: tree.acceptedName,
    authorship: tree.authorship,
    tags,
    whyIncluded: REFERENCE_RULE,
    culturalNote: culturalNote(false, associationState),
    culturalVerified: false,
    facts,
    otherNames: names.filter((name) => name !== display),
    gaps: REFERENCE_GAPS,
    sources,
    tree,
    referenceHref: '/trees',
    referenceLabel: 'Open the tree reference',
  };
}

export function suggestionOption(
  suggestion: RecommendationSuggestion,
  index: number,
  scope: string,
  associationState = ''
): TreeOptionView {
  const scientificName = suggestion.scientificName ?? '';
  // Only call the title botanical when the name and the binomial are the same
  // string: a model that offered no binomial has offered a name, not an identity.
  const titleIsScientific = Boolean(scientificName) && scientificName === suggestion.name;

  const facts: OptionFact[] = [];
  if (scientificName && !titleIsScientific) {
    facts.push({ label: 'Scientific name', value: scientificName });
  }
  if (suggestion.botanicalDetail) facts.push({ label: 'About', value: suggestion.botanicalDetail });
  // Mature size decides whether a suggestion is even possible in the visitor's
  // space, so it sits above the care notes rather than among them. A model that
  // said "not sure" is showing the same honesty the card asks of it.
  const matureSize = (suggestion.matureSize ?? '').trim();
  if (matureSize) facts.push({ label: 'Mature size', value: matureSize });
  for (const requirement of suggestion.growingRequirements ?? []) {
    facts.push({ label: 'Growing', value: requirement });
  }
  for (const limitation of suggestion.limitations ?? []) {
    facts.push({ label: 'Watch out', value: limitation });
  }

  const modelNote = (suggestion.culturalNote ?? '').trim();
  const tags: OptionTag[] = [];
  if (suggestion.identity?.status === 'confirmed') {
    tags.push({ text: 'Taxonomy verified (GBIF)', tone: 'neutral' });
  }
  if (suggestion.sources && suggestion.sources.length > 0) {
    tags.push({ text: 'Backed by sources', tone: 'neutral' });
  } else {
    tags.push({ text: 'AI-generated suggestion', tone: 'pending' });
  }

  const culturalVerified = suggestion.culturalSignificance?.evidence === 'sourced';

  const sources: OptionSource[] = (suggestion.sources ?? []).map((s) => ({
    title: s.title,
    url: s.url,
    meta: s.publisher ? `${s.publisher} · ${s.kind.replace(/_/g, ' ')}` : s.kind.replace(/_/g, ' '),
    tags: [s.kind.replace(/_/g, ' ')],
    attribution: s.publisher ?? '',
  }));

  return {
    key: `${scope}-suggestion-${index}`,
    title: suggestion.name,
    titleIsScientific,
    mark: (suggestion.name || '?').charAt(0).toUpperCase(),
    scientificName,
    authorship: '',
    tags,
    whyIncluded: suggestion.whyItMatches,
    culturalNote:
      suggestion.culturalSignificance?.text || modelNote || culturalNote(false, associationState),
    culturalVerified,
    facts,
    otherNames: [],
    gaps: SUGGESTION_GAPS,
    sources,
    tree: null,
    referenceHref: '/trees',
    referenceLabel: 'Open the tree reference',
  };
}

/** A reviewed cultural association, exactly as the review workflow published it. */
export function reviewedOption(rec: Recommendation, index: number): TreeOptionView {
  const candidate = rec.botanicalCandidates?.[0];
  const named = Boolean(candidate?.scientificName);
  const label = named ? candidate.scientificName : 'Recorded association';

  const facts: OptionFact[] = [];
  if (rec.tradition) facts.push({ label: 'Tradition', value: rec.tradition });
  if (rec.nakshatra) facts.push({ label: 'Nakshatra', value: rec.nakshatra });
  facts.push({ label: 'Region', value: rec.region || 'Not tied to one region' });
  if (rec.version) facts.push({ label: 'Record version', value: String(rec.version) });
  if (rec.interpretation) facts.push({ label: 'Interpretation', value: rec.interpretation });
  for (const c of rec.botanicalCandidates ?? []) {
    facts.push({ label: 'Species', value: `${c.scientificName} — ${candidateMeta(c)}` });
  }

  const tags: OptionTag[] = [{ text: 'Reviewed association' }];
  if (candidate) {
    tags.push({ text: humanise(candidate.reviewStatus), tone: reviewTone(candidate.reviewStatus) });
  } else {
    tags.push({ text: 'species not identified', tone: 'unknown' });
  }

  return {
    key: `record-${rec.revisionId || rec.mappingId || 'association'}-${index}`,
    title: label,
    titleIsScientific: named,
    mark: named ? label.charAt(0).toUpperCase() : '◈',
    scientificName: '',
    authorship: '',
    tags,
    whyIncluded: rec.claim || 'A reviewed record is on file for this selection.',
    culturalNote: culturalNote(true, 'reviewed_match'),
    culturalVerified: true,
    facts,
    otherNames: [],
    gaps: uncertaintyNotes(rec),
    sources: (rec.sources ?? []).map((source) => ({
      title: sourceTitle(source),
      url: validURL(source.url) ? source.url : '',
      meta: [
        source.bibliographicReference,
        source.locator ? `locator: ${source.locator}` : '',
        source.relationship ? `supports the claim: ${humanise(source.relationship)}` : '',
      ]
        .filter(Boolean)
        .join(' · '),
      tags: sourceTags(source),
      attribution: source.attributionRequirements
        ? `Attribution required: ${source.attributionRequirements}`
        : '',
    })),
    tree: null,
    referenceHref: '',
    referenceLabel: '',
  };
}
