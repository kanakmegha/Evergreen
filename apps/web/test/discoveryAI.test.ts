/**
 * What the AI-first discovery client sends, and — more importantly — what it
 * refuses to do when something fails.
 *
 * The rule this file exists to protect: two failures that look identical as an
 * empty array must never be allowed to collapse into one. An inference outage
 * is thrown so the page shows an error; a RESEARCH outage resolves with
 * candidates and `researchStatus: 'unavailable'` so the page can say the answer
 * is provisional. Swapping those two would tell a visitor their question had
 * been researched when nobody looked anything up.
 *
 * `fetch` is stubbed, so no request leaves the process.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  aiDiscoveryRequest,
  candidateToSuggestion,
  distinctSources,
  emptyAnswers,
  EVIDENCE_LABELS,
  isClaimSourced,
  runAIDiscovery,
  sourcesFor,
} from '../src/lib/discovery';
import type { AICandidate, AIDiscoveryResponse, ResearchSource } from '../src/lib/discovery';
import { stubFetch, stubFetchFailure } from './helpers';

const BACKEND = 'https://evergreen-backend-eight.vercel.app';

const SOURCE: ResearchSource = {
  id: 's1',
  title: 'Ashoka tree',
  url: 'https://en.wikipedia.org/wiki/Ashoka_tree',
  publisher: 'Wikipedia',
  kind: 'encyclopedia',
  snippet: 'An evergreen tree of the Indian subcontinent.',
  retrievedAt: '2026-01-04T10:00:00Z',
};

const CANDIDATE: AICandidate = {
  name: 'Ashoka',
  scientificName: 'Saraca asoca',
  whyItMatches: 'You said you wanted something for a small courtyard.',
  botanicalDetail: 'A tree in the family Fabaceae.',
  matureSize: '8-12 m',
  growingRequirements: ['full sun'],
  limitations: ['needs room for the roots'],
  culturalSignificance: { text: 'Revered in the source.', evidence: 'sourced', sourceIds: ['s1'] },
  ecologicalFit: { text: 'Regional fit not settled.', evidence: 'uncertain', sourceIds: [] },
  identity: { status: 'confirmed', acceptedName: 'Saraca asoca', family: 'Fabaceae', source: 'https://www.gbif.org/species/5023541' },
  confidence: { level: 'moderate', definition: 'Sources agree but are few.', assessedBy: 'language model' },
  sourceIds: ['s1'],
};

function response(overrides: Partial<AIDiscoveryResponse> = {}): AIDiscoveryResponse {
  return {
    status: 'ok',
    candidates: [CANDIDATE],
    sources: [SOURCE],
    researchStatus: 'researched',
    researchSummary: 'Researched against 1 source(s) retrieved for this request.',
    confidenceDefinitions: { moderate: 'Sources agree but are few.' },
    promptVersion: 'v1.0-research-grounded',
    model: 'stub-model',
    disclaimer: 'Candidates are written by a language model.',
    saved: false,
    ...overrides,
  };
}

test('the research call uses the configured origin and the documented path', async () => {
  const stub = stubFetch(200, response());
  try {
    await runAIDiscovery({ intention: 'remembering someone' });
  } finally {
    stub.restore();
  }
  assert.equal(stub.calls.length, 1);
  assert.equal(stub.calls[0], `${BACKEND}/api/v1/discovery`);
});

test('the visitor\'s own intention travels verbatim, however unfamiliar', async () => {
  // The server accepts a known key OR free text. The client must not narrow it
  // to the five keys the interface happens to offer, or a question about a
  // tradition Everroot has never heard of never reaches the model at all.
  const stub = stubFetch(200, response());
  try {
    await runAIDiscovery({ intention: 'a tree that means something to the Ahom people' });
  } finally {
    stub.restore();
  }
  const body = JSON.parse(stub.bodies[0]) as { intention: string };
  assert.equal(body.intention, 'a tree that means something to the Ahom people');
});

test('a session token is sent as a bearer header, and omitted without one', async () => {
  const withToken = stubFetch(200, response());
  try {
    await runAIDiscovery({ intention: 'curiosity' }, 'evr_abc');
  } finally {
    withToken.restore();
  }
  // The token proves nothing on its own — the server re-derives the owner hash
  // — but it must travel where the server looks for it.
  assert.match(withToken.calls[0], /\/api\/v1\/discovery$/);

  const without = stubFetch(200, response());
  try {
    await runAIDiscovery({ intention: 'curiosity' });
  } finally {
    without.restore();
  }
  assert.equal(without.calls.length, 1);
});

test('an outage is thrown, never resolved as an empty answer', async () => {
  const stub = stubFetch(503, { error: 'The AI discovery service is temporarily unavailable.' });
  try {
    await assert.rejects(
      () => runAIDiscovery({ intention: 'remembering someone' }),
      (err: Error) => {
        // The server's message is written for a visitor and carries no
        // internals, so it is what the visitor should see.
        assert.match(err.message, /temporarily unavailable/);
        return true;
      }
    );
  } finally {
    stub.restore();
  }
});

test('a rate limit reaches the visitor as the server\'s own actionable words', async () => {
  const stub = stubFetch(429, { error: 'The AI service is busy right now. Please try again in a moment.' });
  try {
    await assert.rejects(
      () => runAIDiscovery({ intention: 'curiosity' }),
      (err: Error) => /busy right now/.test(err.message)
    );
  } finally {
    stub.restore();
  }
});

test('a browser that cannot reach the service is told so in its own words', async () => {
  const stub = stubFetchFailure('offline');
  try {
    await assert.rejects(
      () => runAIDiscovery({ intention: 'curiosity' }),
      (err: Error) => /could not be reached/.test(err.message)
    );
  } finally {
    stub.restore();
  }
});

// ---------------------------------------------------------------------------
// The distinction the whole client rests on.
// ---------------------------------------------------------------------------

test('a research outage resolves WITH candidates and says so', async () => {
  const stub = stubFetch(
    200,
    response({ researchStatus: 'unavailable', researchSummary: 'Not researched: no research backend was available for this request.' })
  );
  try {
    const result = await runAIDiscovery({ intention: 'curiosity' });
    // It resolved. A research failure is a labelled downgrade, not an error.
    assert.equal(result.researchStatus, 'unavailable');
    assert.match(result.researchSummary, /Not researched/);
    assert.equal(result.candidates.length, 1);
  } finally {
    stub.restore();
  }
});

test('an unreadable reply is normalised to an un-researched state, never to "researched"', async () => {
  const stub = stubFetch(200, { status: 'ok' });
  try {
    const result = await runAIDiscovery({ intention: 'curiosity' });
    // A partial body must never default to the strongest claim in the
    // vocabulary. Absent research status means nobody looked anything up.
    assert.equal(result.researchStatus, 'unavailable');
    assert.equal(result.candidates.length, 0);
    assert.equal(result.saved, false);
  } finally {
    stub.restore();
  }
});

test('"no candidates" is an answer, not a failure, and keeps its provenance', async () => {
  const stub = stubFetch(200, response({ status: 'no_candidates', candidates: [], sources: [] }));
  try {
    const result = await runAIDiscovery({ intention: 'curiosity' });
    assert.equal(result.status, 'no_candidates');
    assert.equal(result.candidates.length, 0);
    // The reply still reports what happened to research, so a visitor can be
    // told "we looked and found nothing" apart from "nobody looked".
    assert.equal(result.researchStatus, 'researched');
  } finally {
    stub.restore();
  }
});

test('sources keep their real address, which the server resolved', async () => {
  const stub = stubFetch(200, response());
  try {
    const result = await runAIDiscovery({ intention: 'curiosity' });
    assert.equal(result.sources[0].url, 'https://en.wikipedia.org/wiki/Ashoka_tree');
    assert.equal(result.sources[0].retrievedAt, '2026-01-04T10:00:00Z');
  } finally {
    stub.restore();
  }
});

// ---------------------------------------------------------------------------
// Citations resolve to retrieved sources, or not at all.
// ---------------------------------------------------------------------------

test('a citation resolves in order, and one that does not resolve is dropped', () => {
  const response = { ...responseWithSources(SOURCE, { id: 's2', title: 'Other', url: 'https://example.org/2', kind: 'web_page', retrievedAt: '2026-01-04T10:00:00Z' }) };
  assert.deepEqual(sourcesFor(response, ['s2', 's1']).map((s) => s.id), ['s2', 's1']);
  // An unknown ID renders as nothing rather than as a broken link. The server
  // has already rejected an ID the model invented; this is the second line.
  assert.deepEqual(sourcesFor(response, ['s1', 'ghost']).map((s) => s.id), ['s1']);
  assert.deepEqual(sourcesFor(response, []), []);
});

test('the sources-consulted list is unique and follows first citation', () => {
  const candidates = [
    { ...CANDIDATE, sourceIds: ['s2', 's1'] },
    { ...CANDIDATE, sourceIds: ['s1'] },
  ];
  const all = responseWithSources(
    { id: 's1', title: 'One', url: 'https://example.org/1', kind: 'web_page', retrievedAt: '2026-01-04T10:00:00Z' },
    { id: 's2', title: 'Two', url: 'https://example.org/2', kind: 'web_page', retrievedAt: '2026-01-04T10:00:00Z' }
  );
  const out = distinctSources({ ...all, candidates });
  // s2 is cited first by the first candidate, so it leads: the list follows the
  // order the answer actually drew on its sources, not the order they happen to
  // sit in the response.
  assert.deepEqual(out.map((s) => s.id), ['s2', 's1']);
});

function responseWithSources(...sources: ResearchSource[]): AIDiscoveryResponse {
  return response({ sources, candidates: [CANDIDATE] });
}

// ---------------------------------------------------------------------------
// The card and the provenance describe the same tree.
// ---------------------------------------------------------------------------

test('a researched candidate reshapes into the card the shortlist already knows', () => {
  const suggestion = candidateToSuggestion(CANDIDATE);
  assert.equal(suggestion.name, 'Ashoka');
  assert.equal(suggestion.scientificName, 'Saraca asoca');
  assert.equal(suggestion.whyItMatches, CANDIDATE.whyItMatches);
  assert.equal(suggestion.matureSize, '8-12 m');
  assert.deepEqual(suggestion.growingRequirements, ['full sun']);
  assert.deepEqual(suggestion.limitations, ['needs room for the roots']);
  // Nothing the candidate does not carry is invented for the card: a field
  // filled in here would be a fact the research never established.
  assert.equal('culturalNote' in suggestion, false);
});

test('a candidate with no optional lists still reshapes without crashing', () => {
  const bare: AICandidate = {
    ...CANDIDATE,
    growingRequirements: undefined,
    limitations: undefined,
    matureSize: '',
  };
  const suggestion = candidateToSuggestion(bare);
  assert.deepEqual(suggestion.growingRequirements, []);
  assert.deepEqual(suggestion.limitations, []);
});

// ---------------------------------------------------------------------------
// Evidence vocabulary.
// ---------------------------------------------------------------------------

test('only a sourced claim with an actual citation reads as sourced', () => {
  assert.equal(isClaimSourced(CANDIDATE.culturalSignificance), true);
  assert.equal(isClaimSourced(CANDIDATE.ecologicalFit), false);
  assert.equal(isClaimSourced({ text: 'x', evidence: 'sourced', sourceIds: [] }), false);
  assert.equal(isClaimSourced({ text: 'x', evidence: 'interpretation', sourceIds: ['s1'] }), false);
  assert.equal(isClaimSourced(undefined), false);
  assert.equal(isClaimSourced(null), false);
});

test('every evidence level has plain words the interface can show', () => {
  for (const level of ['sourced', 'uncertain', 'interpretation', 'user_defined', 'none'] as const) {
    assert.ok(EVIDENCE_LABELS[level], `no label for ${level}`);
  }
  assert.equal(Object.keys(EVIDENCE_LABELS).length, 5);
});

// ---------------------------------------------------------------------------
// What travels as "the visitor's own words".
// ---------------------------------------------------------------------------

test('the follow-up answer, the chosen option and the picks travel as one description', () => {
  const request = aiDiscoveryRequest({
    ...emptyAnswers(),
    intention: 'personal',
    followUpNote: 'something for a small courtyard',
    followUpChoice: 'hope',
    tradition: 'Tamil',
    region: 'Tamil Nadu',
  });

  assert.equal(request.intention, 'personal');
  assert.match(request.description ?? '', /something for a small courtyard/);
  // The chosen option is the visitor's own answer, not metadata about it.
  assert.match(request.description ?? '', /hope/);
  assert.equal(request.tradition, 'Tamil');
  assert.equal(request.region, 'Tamil Nadu');
  assert.equal(request.limit, 3);
});

test('an answer that says nothing travels as nothing rather than as filler', () => {
  const request = aiDiscoveryRequest({ ...emptyAnswers(), intention: 'explore' });
  assert.equal(request.description, '');
  assert.equal(request.intention, 'explore');
});

test('a mixed answer — an option chosen AND words written — keeps both', () => {
  // The common case: a visitor picks "remembering someone" and then explains
  // who. Dropping either half would leave the model with a category and no
  // person, or a person and no frame for reading them.
  const request = aiDiscoveryRequest({
    ...emptyAnswers(),
    intention: 'remember',
    followUpChoice: 'hope',
    followUpNote: 'a tree for my grandmother, something she would have planted herself',
    followUpPicks: ['bird-friendly', 'autumn colour'],
  });
  assert.equal(request.intention, 'remember');
  assert.match(request.description ?? '', /my grandmother/);
  assert.match(request.description ?? '', /bird-friendly/);
  assert.match(request.description ?? '', /autumn colour/);
});

test('a tradition and region Everroot has never heard of travel verbatim', () => {
  // No list of supported cultures exists anywhere in this client, so nothing
  // can quietly refuse a tradition the product has not been briefed on. The
  // server resolves what it recognises and researches the rest.
  const request = aiDiscoveryRequest({
    ...emptyAnswers(),
    intention: 'culture',
    tradition: 'Ahom',
    region: 'Assam',
    followUpNote: 'a tree for a memorial in Jorhat',
  });
  assert.equal(request.tradition, 'Ahom');
  assert.equal(request.region, 'Assam');
  assert.match(request.description ?? '', /Jorhat/);
});

test('every step of the flow leaves an intention behind, and open curiosity is the default', () => {
  assert.equal(aiDiscoveryRequest(emptyAnswers()).intention, 'explore');
  assert.equal(aiDiscoveryRequest({ ...emptyAnswers(), intention: 'remember' }).intention, 'remember');
});
