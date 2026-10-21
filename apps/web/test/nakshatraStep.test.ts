/**
 * Step 3 — nakshatra selection — is optional, and the lists that feed the
 * cultural steps are options rather than prerequisites.
 *
 * `test/run-tests.mjs` compiles this file as a production build with the real
 * backend origin in PUBLIC_API_URL, so the URLs asserted here are the ones a
 * visitor's browser would actually use. `fetch` is stubbed, so no request
 * leaves the process.
 *
 * Two properties are under test. First, skipping Step 3 is a real path: the
 * step only appears when it can reach a reviewed association, and a visitor
 * who passes it still produces a complete recommendation request. Second, a
 * lookup that fails takes only itself down — an outage on the traditions or
 * nakshatras list must not become an outage of discovery.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  emptyAnswers,
  fetchNakshatras,
  fetchRegions,
  fetchTraditions,
  NAKSHATRA_TRADITION,
  pathFor,
  recommendationsRequest,
  runRecommendations,
  usesNakshatra,
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

/** Answer each URL on its own, so one lookup can fail while another succeeds. */
function stubFetchByPath(responses: Record<string, { status: number; body: unknown }>) {
  const calls: string[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    calls.push(url);
    const match = Object.keys(responses).find((path) => url.endsWith(path));
    const chosen = match ? responses[match] : { status: 404, body: { error: 'no route' } };
    return {
      ok: chosen.status >= 200 && chosen.status < 300,
      status: chosen.status,
      json: async () => chosen.body,
    } as Response;
  }) as typeof fetch;
  return {
    calls,
    restore() {
      globalThis.fetch = original;
    },
  };
}

/* ------------------------------------------------- Step 3 is not mandatory */

test('the nakshatra step only appears where it can reach a reviewed association', () => {
  const cultural = { ...emptyAnswers(), intention: 'culture' as const, followUpChoice: 'notes' };
  assert.equal(usesNakshatra(cultural), false);
  assert.ok(!pathFor(cultural).includes('nakshatra'));

  const personal = { ...emptyAnswers(), intention: 'personal' as const };
  assert.ok(!pathFor(personal).includes('nakshatra'), 'a personal intention never asks');

  const browsing = { ...emptyAnswers(), intention: 'explore' as const, tradition: null };
  assert.ok(!pathFor(browsing).includes('nakshatra'), 'choosing no tradition never asks');

  const otherTradition = {
    ...emptyAnswers(),
    intention: 'explore' as const,
    tradition: 'some-other-tradition',
  };
  assert.ok(!pathFor(otherTradition).includes('nakshatra'));

  const nakshatraPath = {
    ...emptyAnswers(),
    intention: 'explore' as const,
    tradition: NAKSHATRA_TRADITION,
  };
  assert.equal(usesNakshatra(nakshatraPath), true);
  assert.ok(pathFor(nakshatraPath).includes('nakshatra'), 'the association path still asks');
});

test('skipping Step 3 still sends a complete recommendation request', () => {
  const answers = { ...emptyAnswers(), intention: 'explore' as const, tradition: null };
  const request = recommendationsRequest(answers);

  assert.equal(request.intention, 'explore');
  assert.equal(request.tradition, '');
  assert.equal(request.nakshatra, '');
  assert.equal(request.region, '');
  assert.ok(request.answer !== undefined, 'the visitor\'s own words still travel');
});

test('a visitor who skipped Step 3 gets suggestions anyway', async () => {
  const stub = stubFetch(200, { status: 'ok', suggestions: [], evidence: EVIDENCE });
  try {
    await runRecommendations(
      recommendationsRequest({ ...emptyAnswers(), intention: 'personal', nakshatra: null })
    );
  } finally {
    stub.restore();
  }
  assert.deepEqual(stub.calls, [`${BACKEND}/api/v1/recommendations`]);
});

/* -------------------------------------------- the lists are options, not gates */

test('the nakshatra list arrives as twenty-seven numbered options', async () => {
  const body = Array.from({ length: 27 }, (_, i) => ({ name: `Nakshatra ${i + 1}`, ordinal: i + 1 }));
  const stub = stubFetch(200, body);
  try {
    const list = await fetchNakshatras(BACKEND);
    assert.equal(list.length, 27);
    assert.deepEqual(
      list.map((n) => n.ordinal),
      Array.from({ length: 27 }, (_, i) => i + 1)
    );
    assert.ok(list.every((n) => n.name.length > 0));
  } finally {
    stub.restore();
  }
});

test('traditions arrive as slug-and-name options', async () => {
  const stub = stubFetch(200, [{ slug: NAKSHATRA_TRADITION, name: 'Nakshatra tree association' }]);
  try {
    const list = await fetchTraditions(BACKEND);
    assert.equal(list.length, 1);
    assert.equal(list[0].slug, NAKSHATRA_TRADITION);
    assert.ok(list[0].name.length > 0);
  } finally {
    stub.restore();
  }
});

test('an unrecognised tradition yields an empty region list, not an error', async () => {
  const stub = stubFetch(200, []);
  try {
    const regions = await fetchRegions('not-a-tradition', BACKEND);
    // The region field only narrows a query, so an empty answer has to be
    // something a template can iterate without special-casing null.
    assert.ok(Array.isArray(regions));
    assert.equal(regions.length, 0);
  } finally {
    stub.restore();
  }
});

/* --------------------------------------- a failed lookup does not take all with it */

test('an outage on one cultural list does not reach the recommendations', async () => {
  const stub = stubFetchByPath({
    '/api/v1/traditions': { status: 503, body: { error: 'the reviewed catalogue is unavailable' } },
    '/api/v1/nakshatras': { status: 503, body: { error: 'the reviewed catalogue is unavailable' } },
    '/api/v1/recommendations': {
      status: 200,
      body: {
        status: 'ok',
        suggestions: [{ name: 'Neem', scientificName: 'Azadirachta indica', whyItMatches: 'x' }],
        evidence: EVIDENCE,
        promptVersion: 'v1.4-visitor-anchored-reasons',
        disclaimer: 'Machine-written from the visitor\'s own answers.',
        model: 'stub-model',
      },
    },
  });
  try {
    await assert.rejects(fetchTraditions(BACKEND), /could not|unavailable|failed/i);
    await assert.rejects(fetchNakshatras(BACKEND), /could not|unavailable|failed/i);

    const result = await runRecommendations(
      recommendationsRequest({ ...emptyAnswers(), intention: 'personal' })
    );
    assert.equal(result.status, 'ok');
    assert.equal(result.suggestions.length, 1, 'the recommendation still arrived');
  } finally {
    stub.restore();
  }
});

test('a failed cultural lookup reports what failed, not that discovery is down', async () => {
  const stub = stubFetch(503, { error: 'the reviewed catalogue is unavailable' });
  try {
    await assert.rejects(fetchNakshatras(BACKEND), (error: Error) => {
      const text = error.message;
      assert.ok(
        /503/.test(text),
        `the message should say which status answered, got: ${text}`
      );
      assert.ok(
        !/\bdiscovery\b.*\b(unavailable|down)\b/i.test(text),
        `the message must not declare discovery unavailable, got: ${text}`
      );
      return true;
    });
  } finally {
    stub.restore();
  }
});
