/**
 * What the deployed application sends for a suggestion, and what it refuses to
 * do when the answer does not arrive.
 *
 * `test/run-tests.mjs` compiles this file as a production build with the real
 * backend origin in PUBLIC_API_URL, so the URL asserted here is the one a
 * visitor's browser would actually use. The rule this file exists to protect:
 * a failure is always an exception. The results page may only ever read an
 * empty list as "the model found nothing it would stand behind", never as "the
 * service was down".
 *
 * `fetch` is stubbed, so no request leaves the process.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  emptyAnswers,
  recommendationsRequest,
  runRecommendations,
} from '../src/lib/discovery';
import { stubFetch, stubFetchFailure } from './helpers';

const BACKEND = 'https://evergreen-backend-eight.vercel.app';

const EVIDENCE = {
  culturalAssociation: 'no_reviewed_match',
  botanicalIdentity: 'unavailable',
  habitat: 'no_reviewed_record',
  ecologicalRole: 'no_reviewed_record',
  documentedStories: 'no_reviewed_record',
  personalMeaning: 'user_owned',
};

function request(intention: 'personal' | 'culture' = 'personal') {
  return recommendationsRequest({ ...emptyAnswers(), intention });
}

test('with no argument at all the suggestion call uses the configured origin', async () => {
  const stub = stubFetch(200, { status: 'ok', suggestions: [], evidence: EVIDENCE });
  try {
    await runRecommendations(request());
  } finally {
    stub.restore();
  }
  assert.deepEqual(stub.calls, [`${BACKEND}/api/v1/recommendations`]);
});

test('the body is the request the endpoint accepts, nothing more', async () => {
  const stub = stubFetch(200, { status: 'ok', suggestions: [], evidence: EVIDENCE });
  try {
    await runRecommendations({
      ...request('culture'),
      tradition: 'nakshatra-tree-association',
      nakshatra: 'Rohini',
    });
  } finally {
    stub.restore();
  }

  const [body] = stub.bodies;
  const parsed = JSON.parse(body) as Record<string, unknown>;
  assert.deepEqual(Object.keys(parsed).sort(), [
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
  assert.equal(parsed.intention, 'culture');
  assert.equal(parsed.nakshatra, 'Rohini');
  // The endpoint rejects unknown fields, and the payload is JSON, not a string
  // that could be mistaken for one.
  assert.equal(typeof body, 'string');
});

test('an answer arrives with the suggestions the model made', async () => {
  const stub = stubFetch(200, {
    status: 'ok',
    suggestions: [
      {
        name: 'Neem',
        scientificName: 'Azadirachta indica',
        whyItMatches: 'It suits the courtyard you described.',
        botanicalDetail: 'A tree in the mahogany family.',
        growingRequirements: ['Full sun'],
        limitations: [],
      },
    ],
    evidence: EVIDENCE,
    promptVersion: 'v1.0-structured-suggestions',
    model: 'stub-model',
    disclaimer: 'Machine-written.',
  });
  try {
    const result = await runRecommendations(request());
    assert.equal(result.status, 'ok');
    assert.equal(result.suggestions.length, 1);
    assert.equal(result.suggestions[0].name, 'Neem');
    assert.equal(result.evidence.culturalAssociation, 'no_reviewed_match');
    assert.equal(result.promptVersion, 'v1.0-structured-suggestions');
  } finally {
    stub.restore();
  }
});

test('the optional things reach the page, and an absent one is absent', async () => {
  // matureSize, culturalNote and followUpQuestion are all optional, and the
  // page renders all three — so none of them may be dropped here.
  const full = stubFetch(200, {
    status: 'ok',
    suggestions: [
      {
        name: 'Neem',
        scientificName: 'Azadirachta indica',
        whyItMatches: 'It suits the courtyard you described.',
        botanicalDetail: 'A tree in the mahogany family.',
        matureSize: '15–20 m',
        growingRequirements: ['Full sun'],
        limitations: [],
        culturalNote: 'Everroot has no reviewed record for this tradition and Nakshatra.',
      },
    ],
    evidence: EVIDENCE,
    promptVersion: 'v1.3-visitor-anchored-reasons',
    model: 'stub-model',
    disclaimer: 'Machine-written.',
    followUpQuestion: 'How much direct sun does the spot get?',
  });
  try {
    const result = await runRecommendations(request());
    assert.equal(result.suggestions[0].matureSize, '15–20 m');
    assert.equal(
      result.suggestions[0].culturalNote,
      'Everroot has no reviewed record for this tradition and Nakshatra.'
    );
    assert.equal(result.followUpQuestion, 'How much direct sun does the spot get?');
  } finally {
    full.restore();
  }

  // The common case: no question was needed, so the key is not there at all.
  // The page must read that as "none", not as an empty question to show.
  const quiet = stubFetch(200, { status: 'ok', suggestions: [], evidence: EVIDENCE });
  try {
    const result = await runRecommendations(request());
    assert.equal(result.followUpQuestion, undefined);
    assert.ok(!('followUpQuestion' in result));
  } finally {
    quiet.restore();
  }
});

test('"no suggestions" stays an explicit state rather than a silent empty list', async () => {
  const stub = stubFetch(200, { status: 'no_suggestions', suggestions: [], evidence: EVIDENCE });
  try {
    const result = await runRecommendations(request());
    assert.equal(result.status, 'no_suggestions');
    assert.deepEqual(result.suggestions, []);
  } finally {
    stub.restore();
  }
});

test('an outage is thrown, never resolved as an empty answer', async () => {
  for (const [status, pattern] of [
    [503, /unavailable/i],
    [429, /busy/i],
    [400, /rejected/i],
  ] as const) {
    const stub = stubFetch(status, { error: 'nope' });
    try {
      await assert.rejects(runRecommendations(request()), (error: Error) => {
        assert.match(error.message, pattern);
        return true;
      });
    } finally {
      stub.restore();
    }
  }
});

test('a browser that cannot reach the service is told so in its own words', async () => {
  const stub = stubFetchFailure();
  try {
    await assert.rejects(runRecommendations(request()), /could not be reached from this browser/);
  } finally {
    stub.restore();
  }
});

test('a reply without evidence still reports nothing about culture', async () => {
  const stub = stubFetch(200, { status: 'ok', suggestions: [] });
  try {
    const result = await runRecommendations(request());
    assert.equal(result.evidence.culturalAssociation, 'not_queryable');
    assert.equal(result.evidence.botanicalIdentity, 'unavailable');
  } finally {
    stub.restore();
  }
});
