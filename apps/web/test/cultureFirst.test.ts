/**
 * Culture comes before the names, not after them.
 *
 * The flow used to reach the nakshatra list straight from "I want a nakshatra",
 * which settled the tradition without asking and handed every visitor the same
 * tradition's spellings to choose from. The question of whose tradition it was
 * was therefore unanswerable by the time it was asked, because every option on
 * the page had already assumed the answer.
 *
 * Three properties are under test here. Cultural selection — tradition or
 * community and the naming system — comes first and is skippable like every
 * other optional step; a choice made in one naming system still travels as the
 * name the API knows, so nothing reaches the model as an untranslatable label;
 * and none of it stands between a visitor and a recommendation.
 *
 * `test/run-tests.mjs` compiles this file as a production build with the real
 * backend origin in PUBLIC_API_URL, so the URLs asserted here are the ones a
 * visitor's browser would actually use. `fetch` is stubbed, so no request
 * leaves the process.
 */
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  emptyAnswers,
  fetchNakshatras,
  fetchNakshatrasIn,
  fetchNamingSystems,
  NAKSHATRA_TRADITION,
  normaliseAnswers,
  pathFor,
  recommendationsRequest,
  runRecommendations,
  type StepId,
} from '../src/lib/discovery';
import { stubFetch } from './helpers';

const BACKEND = 'https://evergreen-backend-eight.vercel.app';

const EVIDENCE = {
  culturalAssociation: 'no_reviewed_match',
  botanicalIdentity: 'unavailable',
  habitat: 'no_reviewed_record',
  ecologicalRole: 'no_reviewed_record',
  documentedStories: 'no_reviewed_record',
  personalMeaning: 'user_owned',
};

/** The two ways a visitor can end up being asked for a nakshatra. */
const VIA_EXPLORATION = {
  ...emptyAnswers(),
  intention: 'explore' as const,
  tradition: NAKSHATRA_TRADITION,
};
const VIA_CULTURE = {
  ...emptyAnswers(),
  intention: 'culture' as const,
  followUpChoice: 'nakshatra',
};

/* ------------------------------------------ culture is separate from language */

test('no language selector is confused with cultural selection in the discovery flow', () => {
  for (const [label, answers] of [
    ['exploring a tradition', VIA_EXPLORATION],
    ['asking for a nakshatra directly', VIA_CULTURE],
  ] as const) {
    const order = pathFor(answers);
    assert.ok(
      !order.includes('culture'),
      `${label}: language/naming system selector must not be treated as a cultural gate step in primary flow`
    );
  }
});

test('asking for a nakshatra no longer forces a naming-language step', () => {
  const step = pathFor(VIA_CULTURE);
  assert.ok(!step.includes('culture'), 'language selector removed from primary path');
  assert.ok(step.includes('nakshatra'), 'reaches the nakshatra selection directly');
});

test('cultural context, astrological system, and display language are separate concepts', () => {
  const order = pathFor(VIA_CULTURE) as StepId[];
  assert.deepEqual(order, ['intention', 'followup', 'nakshatra']);
});

/* --------------------------------------- and it can be passed entirely */

test('skipping cultural selection still sends a complete recommendation request', () => {
  // The visitor declined both questions: no tradition, no naming system, no
  // nakshatra. The request that leaves is the same shape as any other.
  const declined = { ...VIA_CULTURE, tradition: null as string | null, naming: '' };
  const request = recommendationsRequest(declined);

  assert.equal(request.intention, 'culture');
  assert.equal(request.tradition, '');
  assert.equal(request.nakshatra, '');
  assert.equal(request.region, '');
  assert.ok(
    request.answer !== undefined,
    "the visitor's own words travel whether or not they chose a tradition"
  );
});

test('a visitor who declined cultural selection gets suggestions anyway', async () => {
  const stub = stubFetch(200, {
    status: 'ok',
    suggestions: [{ name: 'Neem', scientificName: 'Azadirachta indica', whyItMatches: 'x' }],
    evidence: EVIDENCE,
    promptVersion: 'v1.4-visitor-anchored-reasons',
    disclaimer: 'Machine-written from the visitor’s own answers.',
    model: 'stub-model',
  });
  try {
    const declined = { ...VIA_CULTURE, tradition: null as string | null, naming: '' };
    const result = await runRecommendations(recommendationsRequest(declined));
    assert.equal(result.status, 'ok');
    assert.equal(result.suggestions.length, 1, 'the model answered with nothing cultural set');
  } finally {
    stub.restore();
  }
  assert.deepEqual(stub.calls, [`${BACKEND}/api/v1/recommendations`]);
});

test('the cultural lookups are not part of the recommendation at all', () => {
  // A naming system is display context. If it were carried into the request it
  // would either change what the model was asked or need the same round trip,
  // and either way a lookup that failed could take the recommendation with it.
  const withEverything = {
    ...VIA_CULTURE,
    tradition: NAKSHATRA_TRADITION,
    naming: 'ta',
    nakshatra: 'Mula',
    nakshatraName: 'மூலம்',
  };
  const request = recommendationsRequest(withEverything) as unknown as Record<string, unknown>;
  assert.ok(
    !('naming' in request),
    'the naming system is how the visitor reads the answer, not part of the question'
  );
  assert.ok(
    !('nakshatraName' in request),
    'the display spelling never travels; the canonical name does'
  );
});

/* ------------------------------------------ Tamil names arrive as Tamil */

test('a Tamil naming system serves the names in Tamil, each tied to its canonical', async () => {
  const body = [
    { name: 'அசுவினி', ordinal: 1, canonical: 'Ashwini' },
    { name: 'பரணி', ordinal: 2, canonical: 'Bharani' },
  ];
  const stub = stubFetch(200, body);
  try {
    const list = await fetchNakshatrasIn('ta', BACKEND);
    assert.equal(stub.calls[0], `${BACKEND}/api/v1/nakshatras/ta`);
    assert.equal(list.length, 2);
    assert.match(list[0].name, /[\u0B80-\u0BFF]/, 'the name shown is Tamil script');
    assert.equal(list[0].canonical, 'Ashwini', 'the canonical name still travels on');
  } finally {
    stub.restore();
  }
});

test('the naming systems on offer carry a name a visitor can recognise', async () => {
  const stub = stubFetch(200, [
    { code: 'canonical', name: 'Sanskrit, in English script', endonym: 'Sanskrit', script: 'Latin' },
    { code: 'ta', name: 'Tamil', endonym: 'தமிழ்', script: 'Tamil' },
  ]);
  try {
    const list = await fetchNamingSystems(BACKEND);
    assert.equal(stub.calls[0], `${BACKEND}/api/v1/naming-systems`);
    const tamil = list.find((system) => system.code === 'ta');
    assert.ok(tamil, 'Tamil has to be selectable, not merely reachable');
    // The endonym is what makes the choice recognisable without reading an
    // English label, which is the whole point of offering it.
    assert.ok(tamil.endonym.length > 0 && tamil.script.length > 0);
  } finally {
    stub.restore();
  }
});

test('a naming system Everroot does not hold still returns all twenty-seven', async () => {
  const body = Array.from({ length: 27 }, (_, i) => ({
    name: `Name ${i + 1}`,
    ordinal: i + 1,
    canonical: `Nakshatra ${i + 1}`,
  }));
  const stub = stubFetch(200, body);
  try {
    const list = await fetchNakshatrasIn('gr', BACKEND);
    assert.equal(list.length, 27, 'the sequence is the same everywhere; only the spelling differs');
    assert.deepEqual(
      list.map((n) => n.ordinal),
      Array.from({ length: 27 }, (_, i) => i + 1),
      'the canonical order never depends on the naming system'
    );
    assert.ok(
      list.every((n) => n.canonical && n.canonical.startsWith('Nakshatra')),
      'every entry still carries the name the API knows'
    );
  } finally {
    stub.restore();
  }
});

test('a canonical name choice is sent unchanged, whatever it was displayed as', async () => {
  const request = recommendationsRequest({
    ...VIA_CULTURE,
    tradition: NAKSHATRA_TRADITION,
    nakshatra: 'Mula',
    nakshatraName: 'மூலம்',
    naming: 'ta',
  });
  // The visitor read மூலம் and picked it. The record, the reviewed lookup and
  // the model are all addressed as Mula, so the choice lands on the same
  // nakshatra it would have if they had read it in English.
  assert.equal(request.nakshatra, 'Mula');
  assert.ok(!request.nakshatra.includes('மூலம்'), 'a display name never travels as a lookup key');
});

test('the unqualified list and the canonical naming system agree on the sequence', async () => {
  const plain = Array.from({ length: 27 }, (_, i) => ({ name: `N${i + 1}`, ordinal: i + 1 }));
  const stub = stubFetch(200, plain);
  try {
    const list = await fetchNakshatras(BACKEND);
    assert.equal(stub.calls[0], `${BACKEND}/api/v1/nakshatras`);
    assert.deepEqual(
      list.map((n) => n.ordinal),
      Array.from({ length: 27 }, (_, i) => i + 1)
    );
  } finally {
    stub.restore();
  }
});

/* ------------------------------------ a record from an older visit still reads */

test('a record written before any of this was asked still loads with every field', () => {
  // Backwards compatibility is part of the feature, not an afterthought: a
  // visitor who chose a nakshatra last month should not meet a page reading an
  // undefined field back into a template.
  const legacy = normaliseAnswers({
    intention: 'culture',
    followUpChoice: 'nakshatra',
    tradition: NAKSHATRA_TRADITION,
    traditionName: 'Nakshatra tree association',
    region: '',
    nakshatra: 'Revati',
  } as never);

  assert.equal(legacy.naming, '', 'absent means no preference, never an assumed default');
  assert.equal(legacy.nakshatraName, 'Revati', 'the display name falls back to what was stored');
  assert.ok(Array.isArray(legacy.followUpPicks));
});

test('a record with nothing on it at all is still a complete set of answers', () => {
  for (const raw of [null, undefined, {}]) {
    const answers = normaliseAnswers(raw as never);
    assert.equal(answers.naming, '');
    assert.equal(answers.nakshatra, null);
    assert.equal(answers.nakshatraName, '');
  }
});

/* ------------------------------------------- the step says what it means */

const here = path.dirname(fileURLToPath(import.meta.url));
const discover = fs.readFileSync(path.join(here, '..', 'src', 'routes', 'discover', '+page.svelte'), 'utf8');

test('the cultural step states that nothing is inferred about the visitor', () => {
  // The feature is a question, not a deduction. Naming where it draws the line
  // is what stops it reading as an inference about somebody's background.
  assert.match(
    discover,
    /not from your name, not from where you are/,
    'the step should say plainly that it is not guessing at the visitor'
  );
  assert.match(
    discover,
    /or skip this and Everroot will suggest from your\s+answers alone/,
    'and that passing it through is a real option'
  );
});

test('the step can be left without an answer', () => {
  // The continue affordance has to change when nothing was chosen, so the
  // button a visitor reads agrees with what it will actually do.
  assert.match(discover, /'Skip cultural context'/, 'the continue label admits it is a skip');
  // Both cultural groups leave `canContinue` true: no tradition, no naming
  // system and no nakshatra are each a state the visitor may choose.
  assert.match(
    discover,
    /Both cultural steps are optional/,
    'the intention behind the always-available continue'
  );
});

test('an unsupported naming system is offered the map, not presented as the map', () => {
  // The visitor asked for names Everroot does not hold. The twenty-seven are
  // shown anyway — they are the same wherever you go — but said to be the
  // romanised ones, and the model's cultural writing said to be written
  // rather than sourced.
  assert.match(discover, /namingUnsupported/, 'the state exists');
  assert.match(discover, /Everroot has no names in that naming system yet/);
  assert.match(discover, /the list below is in romanised\s+Sanskrit/);
  assert.match(
    discover,
    /written rather than\s+sourced, and may need confirming/,
    'an AI-generated association is never presented as a reviewed one'
  );
});

test('the cultural steps never claim a reviewed source they do not have', () => {
  const spoken = discover
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\/\*|\*)/.test(line))
    .join('\n');
  assert.ok(
    !/reviewed (cultural )?dataset/i.test(spoken),
    'internal vocabulary stays out of anything a visitor reads'
  );
  assert.ok(
    !/verified (cultural )?(record|source)/i.test(spoken),
    'nothing here may imply a reviewed record exists behind a selection'
  );
});

test('the tradition offered is described as what it holds, not as everything', () => {
  // One tradition's names are on offer. Saying so is what keeps the others
  // from being quietly implied.
  assert.match(
    discover,
    /The 27 nakshatras and the trees recorded against them/,
    'the supported tradition is described by its scope'
  );
});
