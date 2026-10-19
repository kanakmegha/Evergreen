/**
 * The request that draws tree suggestions, and the card one is shown on.
 *
 * Both are pure functions, so they are the right place to pin the two rules the
 * rest of this feature leans on: every intention — not only a Nakshatra one —
 * produces a request that carries what the visitor actually said, and a
 * machine-written suggestion can never be dressed up as a reviewed record or
 * borrow a source it does not have.
 *
 * Compiled by `test/run-tests.mjs` as a production build with PUBLIC_API_URL
 * absent, so nothing here depends on a configured origin. `fetch` is never
 * called.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { emptyAnswers, recommendationsRequest } from '../src/lib/discovery';
import type { FlowAnswers } from '../src/lib/discovery';
import {
  SUGGESTION_GAPS,
  SUGGESTION_RULE,
  culturalNote,
  suggestionOption,
} from '../src/lib/treeOption';

function answers(overrides: Partial<FlowAnswers> = {}): FlowAnswers {
  return { ...emptyAnswers(), ...overrides };
}

test('a personal intention reaches the request with nothing cultural attached', () => {
  const request = recommendationsRequest(
    answers({
      intention: 'personal',
      followUpNote: 'Patience — for a year of learning to slow down.',
      followUpPicks: ['A quality I value'],
    })
  );

  assert.equal(request.intention, 'personal');
  assert.equal(request.answer, 'Patience — for a year of learning to slow down.');
  assert.deepEqual(request.picks, ['A quality I value']);
  // No tradition was chosen, so none is asserted on the server's behalf.
  assert.equal(request.tradition, '');
  assert.equal(request.nakshatra, '');
  assert.equal(request.region, '');
  assert.equal(request.limit, 3);
  assert.deepEqual(request.constraints, []);
});

test('every intention in the flow produces a request, none more than another', () => {
  for (const intention of ['personal', 'remember', 'culture', 'future', 'explore'] as const) {
    const request = recommendationsRequest(answers({ intention }));
    assert.equal(request.intention, intention, `${intention} must reach the endpoint`);
    assert.ok(request.limit >= 1, `${intention} must ask for at least one suggestion`);
  }
});

test('the request carries exactly the keys the endpoint accepts', () => {
  // The handler decodes with DisallowUnknownFields, so one stray key from here
  // is a 400 for the visitor. The key set is part of the contract.
  const request = recommendationsRequest(answers({ intention: 'explore' }));
  assert.deepEqual(Object.keys(request).sort(), [
    'answer',
    'choice',
    'constraints',
    'intention',
    'limit',
    'nakshatra',
    'picks',
    'region',
    'tradition',
  ]);
});

test('a cultural selection arrives as context rather than as a requirement', () => {
  const request = recommendationsRequest(
    answers({
      intention: 'culture',
      followUpChoice: 'nakshatra',
      tradition: 'nakshatra-tree-association',
      traditionName: 'Nakshatra tree association',
      region: 'Tamil Nadu',
      nakshatra: 'Rohini',
    })
  );

  assert.equal(request.intention, 'culture');
  assert.equal(request.tradition, 'nakshatra-tree-association');
  assert.equal(request.nakshatra, 'Rohini');
  assert.equal(request.region, 'Tamil Nadu');
  // The follow-up choice is sent as the label a person chose, not as a slug the
  // model would have to decode.
  assert.equal(request.choice, 'Nakshatra or astrological traditions');
});

test('a cultural intention without a tradition is still a complete request', () => {
  // The one thing that must never become mandatory is a Nakshatra selection.
  const request = recommendationsRequest(
    answers({ intention: 'culture', followUpChoice: 'family' })
  );
  assert.equal(request.intention, 'culture');
  assert.equal(request.nakshatra, '');
  assert.equal(request.choice, 'Family or regional customs');
});

test('an intention is always present, including for a record that predates the flow', () => {
  const request = recommendationsRequest(answers({ intention: null }));
  assert.equal(request.intention, 'explore');
  assert.equal(request.answer, '');
  assert.deepEqual(request.picks, []);
});

/* ------------------------------------------------------------- the card */

function suggestion(overrides: Partial<Parameters<typeof suggestionOption>[0]> = {}) {
  return {
    name: 'Neem',
    scientificName: 'Azadirachta indica',
    whyItMatches: 'It suits the shaded courtyard you described.',
    botanicalDetail: 'A tree in the mahogany family, native to the Indian subcontinent.',
    growingRequirements: ['Full sun', 'Deep soil'],
    limitations: ['Fast surface roots'],
    ...overrides,
  };
}

test('a suggestion is labelled as generated and never as a verified cultural record', () => {
  const option = suggestionOption(suggestion(), 0, 'shortlist', 'no_reviewed_match');

  assert.deepEqual(option.tags, [{ text: 'AI-generated suggestion', tone: 'pending' }]);
  assert.equal(option.culturalVerified, false);
  assert.equal(option.culturalNote, culturalNote(false, 'no_reviewed_match'));
  // The gate's verdict, quoted rather than overridden by the card. The wording
  // states what Everroot does not hold; it never argues with the tradition the
  // visitor chose, and never claims there is no association.
  assert.match(option.culturalNote, /no reviewed record for this combination/);
  assert.match(option.culturalNote, /nothing cultural is claimed/);
});

test('the card carries the reason, the botany, the growing needs and the limits', () => {
  const option = suggestionOption(suggestion(), 0, 'shortlist', 'not_queryable');

  assert.equal(option.whyIncluded, 'It suits the shaded courtyard you described.');

  const valuesFor = (label: string) =>
    option.facts.filter((fact) => fact.label === label).map((fact) => fact.value);
  const labelled = Object.fromEntries(option.facts.map((fact) => [fact.label, fact.value]));

  assert.equal(labelled['Scientific name'], 'Azadirachta indica');
  assert.equal(labelled['About'], 'A tree in the mahogany family, native to the Indian subcontinent.');
  // Every growing need and every limitation is kept, not merged into one line.
  assert.deepEqual(valuesFor('Growing'), ['Full sun', 'Deep soil']);
  assert.deepEqual(valuesFor('Watch out'), ['Fast surface roots']);
});

test('a suggestion never borrows a source it does not have', () => {
  const option = suggestionOption(suggestion(), 0, 'shortlist');

  assert.deepEqual(option.sources, []);
  assert.deepEqual(option.gaps, SUGGESTION_GAPS);
  assert.match(option.gaps[0], /^Source — none/);
  // The rule is honest about where the text came from and warm about why:
  // it tells the visitor it was written for their answers, and says plainly
  // that a reviewed record is not what stands behind it.
  assert.match(SUGGESTION_RULE, /your answers/);
  assert.match(SUGGESTION_RULE, /written for your situation/);
  assert.match(SUGGESTION_RULE, /not read from a reviewed record/);
  // And it never argues with the choice the visitor just made.
  assert.doesNotMatch(SUGGESTION_RULE, /never a cultural association/);
});

test('the title is only treated as botanical when it is the binomial itself', () => {
  const common = suggestionOption(suggestion(), 0, 'shortlist');
  assert.equal(common.titleIsScientific, false);
  assert.equal(common.title, 'Neem');

  const bare = suggestionOption(
    suggestion({ name: 'Shorea robusta', scientificName: 'Shorea robusta' }),
    1,
    'shortlist'
  );
  assert.equal(bare.titleIsScientific, true);
  assert.ok(!bare.facts.some((fact) => fact.label === 'Scientific name'));

  const unnamed = suggestionOption(
    suggestion({ name: 'Unnamed suggestion', scientificName: '' }),
    2,
    'shortlist'
  );
  assert.equal(unnamed.titleIsScientific, false);
  assert.equal(unnamed.scientificName, '');
});

test('each suggestion gets its own key, so a list can be updated in place', () => {
  const first = suggestionOption(suggestion(), 0, 'shortlist');
  const second = suggestionOption(suggestion(), 1, 'shortlist');
  assert.notEqual(first.key, second.key);
  assert.match(first.key, /^shortlist-suggestion-/);
});

test('mature size sits above the care notes, and an honest "not sure" is left alone', () => {
  const sized = suggestionOption(suggestion({ matureSize: '15–20 m' }), 0, 'shortlist');
  const labelled = Object.fromEntries(sized.facts.map((fact) => [fact.label, fact.value]));
  assert.equal(labelled['Mature size'], '15–20 m');
  // It decides whether the tree is even possible in the visitor's space, so it
  // comes before the growing needs rather than among them.
  const labels = sized.facts.map((fact) => fact.label);
  assert.ok(labels.indexOf('Mature size') < labels.indexOf('Growing'));

  // A model that said it did not know is being honest, not being empty: the
  // row stays and says so, the way every other field on this card does.
  const unsure = suggestionOption(suggestion({ matureSize: 'not sure' }), 0, 'shortlist');
  assert.ok(unsure.facts.some((fact) => fact.label === 'Mature size' && fact.value === 'not sure'));

  // Absent is absent: no blank row, and no placeholder invented by the card.
  const silent = suggestionOption(suggestion({ matureSize: '' }), 0, 'shortlist');
  assert.ok(!silent.facts.some((fact) => fact.label === 'Mature size'));
});

test("the model's one sentence about an unverified request replaces the card's own", () => {
  const option = suggestionOption(
    suggestion({
      culturalNote: 'Everroot has no reviewed record for this tradition and Nakshatra.',
    }),
    0,
    'shortlist',
    'no_reviewed_match'
  );

  assert.equal(option.culturalNote, 'Everroot has no reviewed record for this tradition and Nakshatra.');
  assert.equal(option.culturalVerified, false);
});

test('a missing or blank cultural note falls back to the gate, never to nothing', () => {
  // Most suggestions are drawn without any cultural question at all, and a
  // model that padded its note with whitespace has said nothing. In both cases
  // the server's sentence is the one worth showing.
  for (const culturalNoteValue of ['', '   ', undefined]) {
    const option = suggestionOption(
      suggestion(
        culturalNoteValue === undefined ? {} : { culturalNote: culturalNoteValue }
      ),
      0,
      'shortlist',
      'not_queryable'
    );
    assert.equal(option.culturalNote, culturalNote(false, 'not_queryable'));
  }
});
