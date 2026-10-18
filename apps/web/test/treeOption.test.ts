/**
 * Unit coverage for the option view models and the catalogue filter.
 *
 * These are the pure functions that decide what a card is allowed to say, so
 * they are the right place to assert the guardrail that matters most: a
 * botanical reference record can never present itself as a cultural
 * association, and a missing common name or missing source metadata is shown
 * honestly rather than papered over.
 *
 * Run with `npm test`. The suite uses node:test and the esbuild that already
 * ships with Vite, so it adds no dependency.
 */

import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  REFERENCE_GAPS,
  REFERENCE_RULE,
  candidateMeta,
  culturalNote,
  filterTrees,
  formatConfidence,
  referenceOption,
  reviewedOption,
  sourceTags,
  titleCase,
  uncertaintyNotes,
} from '../src/lib/treeOption';
import type { Recommendation, TreeFact } from '../src/lib/discovery';

const GBIF = {
  dataset: 'GBIF Backbone Taxonomy',
  doi: '10.15468/39omei',
  url: 'https://www.gbif.org/species/3189866',
  taxonKey: 3189866,
};

function tree(overrides: Partial<TreeFact> = {}): TreeFact {
  return {
    speciesId: '11111111-1111-1111-1111-111111111111',
    acceptedName: 'Mangifera indica',
    authorship: 'L.',
    family: 'Anacardiaceae',
    genus: 'Mangifera',
    order: 'Sapindales',
    rank: 'SPECIES',
    taxonStatus: 'ACCEPTED',
    protologue: 'Sp. Pl.: 200 (1753)',
    commonNames: [
      { name: 'common mango', language: 'eng' },
      { name: 'Indian mango', language: 'eng' },
    ],
    authority: GBIF,
    sources: [
      {
        title: 'GBIF Backbone Taxonomy',
        url: 'https://www.gbif.org/species/3189866',
        verificationStatus: 'verified',
        reuseStatus: 'cc-by-4.0',
        attributionRequirements: 'CC BY 4.0 — GBIF',
        relationship: 'IDENTIFIES_SPECIES',
        bibliographicReference: 'GBIF Secretariat (2023)',
        locator: 'taxonKey 3189866',
      },
    ],
    recordType: 'botanical_authority',
    culturalClaim: 'none',
    ...overrides,
  } as TreeFact;
}

function recommendation(overrides: Partial<Recommendation> = {}): Recommendation {
  return {
    mappingId: '22222222-2222-2222-2222-222222222222',
    revisionId: '33333333-3333-3333-3333-333333333333',
    version: 3,
    claim: 'A reviewed claim about the tree and the Nakshatra.',
    interpretation: 'How the record has been read.',
    tradition: 'IN-TN',
    nakshatra: 'Rohini',
    region: 'IN-TN',
    sources: [
      {
        title: 'A reviewed source',
        url: 'https://example.org/source',
        verificationStatus: 'verified',
        reuseStatus: 'cleared',
        attributionRequirements: '',
        relationship: 'SUPPORTS_CLAIM',
        bibliographicReference: 'Author (2001)',
        locator: 'p. 42',
      },
    ],
    botanicalCandidates: [
      {
        scientificName: 'Ficus religiosa',
        certainty: 'strong',
        confidence: 0.82,
        definitive: true,
        reviewStatus: 'verified',
      },
    ],
    ...overrides,
  } as Recommendation;
}

/* ------------------------------------------------- a reviewed association */

test('a reviewed association is labelled as one, with its own sources', () => {
  const option = reviewedOption(recommendation(), 0);

  assert.equal(option.title, 'Ficus religiosa');
  assert.equal(option.titleIsScientific, true);
  assert.equal(option.culturalVerified, true);
  assert.deepEqual(option.tags[0], { text: 'Reviewed association' });
  // The claim is reproduced, not paraphrased, and never generated here.
  assert.equal(option.whyIncluded, 'A reviewed claim about the tree and the Nakshatra.');
  assert.equal(option.sources.length, 1);
  assert.equal(option.sources[0].url, 'https://example.org/source');
  assert.equal(option.sources[0].attribution, '');
  assert.ok(option.sources[0].tags.includes('verified'));
});

test('a reviewed association with no identified species says so instead of naming one', () => {
  const option = reviewedOption(recommendation({ botanicalCandidates: [] }), 0);

  assert.equal(option.title, 'Recorded association');
  assert.equal(option.titleIsScientific, false);
  assert.equal(option.tags[1].tone, 'unknown');
  assert.ok(option.gaps.some((gap) => gap.includes('No botanical identification')));
});

/* --------------------------------------- a botanical reference record */

test('a reference record carries no cultural claim and labels itself honestly', () => {
  const option = referenceOption(tree(), 0, 'shortlist', true, 'no_reviewed_match');

  assert.equal(option.culturalVerified, false);
  assert.deepEqual(option.tags, [
    { text: 'Botanical record', tone: 'neutral' },
    { text: 'accepted name', tone: 'neutral' },
  ]);
  assert.equal(option.culturalNote, 'Checked against Everroot’s reviewed cultural dataset: no record for this selection.');
  // The rule is a statement about ordering, never a claim about the visitor.
  assert.equal(option.whyIncluded, REFERENCE_RULE);
  assert.deepEqual(option.gaps, REFERENCE_GAPS);
  assert.equal(option.tree?.speciesId, tree().speciesId);
});

test('the selection rule is one or two short sentences and claims no match', () => {
  const words = REFERENCE_RULE.split(/\s+/).filter(Boolean);
  assert.ok(words.length >= 20 && words.length <= 35, `rule was ${words.length} words`);
  assert.match(REFERENCE_RULE, /botanical reference/);
  assert.match(REFERENCE_RULE, /not because it matches your answers/);
  assert.match(REFERENCE_RULE, /not a recommendation/);
  // The cultural point is made once at the top of the page, not on every card.
  assert.doesNotMatch(REFERENCE_RULE, /cultural/i);
});

test('a common name becomes the title and the rest are listed as alternates', () => {
  const option = referenceOption(tree(), 0, 'shortlist', true);

  // The two recorded names are the same length, so the tie breaks alphabetically.
  // Nothing is invented, and every recorded name is still shown.
  assert.equal(option.title, 'Common mango');
  assert.equal(option.titleIsScientific, false);
  assert.equal(option.scientificName, 'Mangifera indica');
  assert.deepEqual(option.otherNames, ['Indian mango']);
  assert.equal(option.mark, 'M');
});

test('a missing common name falls back to the scientific name without a placeholder', () => {
  const option = referenceOption(tree({ commonNames: [] }), 0, 'shortlist', true);

  assert.equal(option.title, 'Mangifera indica');
  assert.equal(option.titleIsScientific, true);
  assert.deepEqual(option.otherNames, []);
  // Nothing anywhere implies a name is missing because of a fault.
  assert.doesNotMatch(option.title, /unknown|n\/a|—/i);
});

test('the display name is stable whichever order the authority lists names in', () => {
  const forward = referenceOption(
    tree({
      commonNames: [
        { name: 'common mango', language: 'eng' },
        { name: 'Mango', language: 'eng' },
      ],
    }),
    0,
    'shortlist',
    true
  );
  const reversed = referenceOption(
    tree({
      commonNames: [
        { name: 'Mango', language: 'eng' },
        { name: 'common mango', language: 'eng' },
      ],
    }),
    0,
    'shortlist',
    true
  );

  assert.equal(forward.title, 'Mango');
  assert.equal(reversed.title, 'Mango');
});

test('the authority source is listed once, with the taxon key kept for checking', () => {
  const option = referenceOption(tree(), 0, 'shortlist', true);

  const titles = option.sources.map((source) => source.title);
  assert.deepEqual(titles, ['GBIF Backbone Taxonomy taxon record']);
  assert.match(option.sources[0].meta, /doi:10\.15468\/39omei/);
  assert.match(option.sources[0].meta, /taxon key 3189866/);
  assert.equal(option.sources[0].url, GBIF.url);
});

test('missing source metadata leaves the source unlinked rather than invented', () => {
  const option = referenceOption(
    tree({
      authority: { dataset: 'GBIF Backbone Taxonomy', doi: '', url: '', taxonKey: 0 },
      sources: [],
    }),
    0,
    'shortlist',
    true
  );

  assert.equal(option.sources.length, 1);
  assert.equal(option.sources[0].url, '');
  assert.equal(option.sources[0].meta, '');
  assert.equal(option.sources[0].tags[0], 'authority match');
});

test('a non-authority source keeps its own verification and licence states', () => {
  const option = referenceOption(
    tree({
      sources: [
        {
          title: 'A herbarium checklist',
          url: 'https://example.org/checklist',
          verificationStatus: 'pending',
          reuseStatus: 'restricted',
          attributionRequirements: 'Contact the publisher',
          relationship: 'IDENTIFIES_SPECIES',
          bibliographicReference: 'Curator (1999)',
          locator: 'entry 12',
        },
      ],
    }),
    0,
    'shortlist',
    true
  );

  const extra = option.sources.find((source) => source.title === 'A herbarium checklist');
  assert.ok(extra, 'the additional source should be listed');
  if (!extra) return;
  assert.deepEqual(extra.tags, ['pending', 'reuse: restricted']);
  assert.equal(extra.attribution, 'Attribution required: Contact the publisher');
  assert.match(extra.meta, /locator: entry 12/);
});

test('catalogue rows are marked as already being in the shortlist', () => {
  const inShortlist = referenceOption(tree(), 0, 'catalogue', true);
  const outside = referenceOption(tree(), 0, 'catalogue', false);

  assert.equal(inShortlist.tags.length, 2);
  assert.equal(outside.tags.at(-1)?.text, 'in your shortlist');
});

/* -------------------------------------------- cultural note and honesty */

test('the cultural note follows the publication gate, not a guess', () => {
  assert.match(culturalNote(false, 'no_reviewed_match'), /no record for this selection/);
  assert.match(culturalNote(false, 'not_queryable'), /no cultural match was checked/);
  assert.match(culturalNote(false, ''), /has not verified/);
  assert.match(culturalNote(true, 'reviewed_match'), /Reviewed and published/);
});

test('uncertainty is derived from the record and never left empty', () => {
  const noSources = uncertaintyNotes(
    recommendation({ sources: [], region: '' })
  );
  assert.ok(noSources.some((note) => note.includes('No source is attached')));
  assert.ok(noSources.some((note) => note.includes('not tied to one region')));

  const partiallyReviewed = uncertaintyNotes(
    recommendation({
      botanicalCandidates: [
        { scientificName: 'A', certainty: 'likely', confidence: 0.5, definitive: false, reviewStatus: 'verified' },
        { scientificName: 'B', certainty: 'likely', confidence: 0.4, definitive: true, reviewStatus: 'verified' },
      ] as Recommendation['botanicalCandidates'],
    })
  );
  assert.ok(partiallyReviewed.some((note) => note.includes('1 of 2')));

  const clean = uncertaintyNotes(recommendation());
  assert.equal(clean.length, 1);
  assert.match(clean[0], /marked verified/);
});

/* --------------------------------------------------- small formatters */

test('titles, confidences and source tags are formatted predictably', () => {
  assert.equal(titleCase('common mango'), 'Common mango');
  assert.equal(titleCase(''), '');
  assert.equal(formatConfidence(0.82), '82% confidence');
  assert.equal(formatConfidence(82), '82% confidence');
  assert.match(formatConfidence(Number.NaN), /not recorded/);
  assert.equal(candidateMeta({ certainty: '', confidence: Number.NaN, definitive: false } as never), 'unknown · confidence not recorded · not definitive');
  assert.deepEqual(sourceTags({ verificationStatus: 'verified', reuseStatus: 'cleared' } as never), [
    'verified',
    'reuse: cleared',
  ]);
});

test('the rank is shown as a readable label, not as the authority code', () => {
  const species = referenceOption(tree({ rank: 'SPECIES' }), 0, 'shortlist', true);
  assert.deepEqual(
    species.facts.find((fact) => fact.label === 'Rank'),
    { label: 'Rank', value: 'Species' }
  );
  const variety = referenceOption(tree({ rank: 'VARIETY' }), 0, 'shortlist', true);
  assert.equal(variety.facts.find((fact) => fact.label === 'Rank')?.value, 'Variety');
});

/* ------------------------------------------------- catalogue filtering */

test('the catalogue filter matches common names, scientific names and families', () => {
  const trees = [
    tree(),
    tree({
      speciesId: '44444444-4444-4444-4444-444444444444',
      acceptedName: 'Azadirachta indica',
      genus: 'Azadirachta',
      family: 'Meliaceae',
      commonNames: [{ name: 'neem', language: 'eng' }],
    }),
  ];

  assert.equal(filterTrees(trees, '').length, 2);
  assert.equal(filterTrees(trees, '   ').length, 2);
  assert.deepEqual(
    filterTrees(trees, 'neem').map((entry) => entry.acceptedName),
    ['Azadirachta indica']
  );
  assert.deepEqual(
    filterTrees(trees, 'MELIACEAE').map((entry) => entry.acceptedName),
    ['Azadirachta indica']
  );
  assert.deepEqual(
    filterTrees(trees, 'mangifera').map((entry) => entry.acceptedName),
    ['Mangifera indica']
  );
  assert.equal(filterTrees(trees, 'quercus').length, 0);
});

test('an empty catalogue filters to nothing rather than throwing', () => {
  assert.deepEqual(filterTrees([], 'neem'), []);
  assert.deepEqual(filterTrees([], ''), []);
});
