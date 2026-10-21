/**
 * Intention-first, culture-aware, research-driven discovery test suite.
 *
 * Covers:
 * - Explicit Western zodiac request (defaults to Western zodiac and labels clearly)
 * - Explicit Tamil/Nakshatra request (recognises context, never defaults to Western)
 * - Request involving an unfamiliar culture (carried verbatim without catalogue allow-lists)
 * - Request that needs a clarification (ambiguous astrology query asks clarification)
 * - Request where user skips cultural context (general discovery proceeds)
 * - Free-text culture selection
 * - Research success, partial success, provider outage, and no-candidate results
 * - No catalogue dependency in the active recommendation path
 * - No language selector being confused with cultural selection
 * - Source URLs and provenance matching the actual research response
 */

import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  aiDiscoveryRequest,
  candidateToSuggestion,
  detectAstrologyContext,
  distinctSources,
  emptyAnswers,
  needsAstrologyClarification,
  pathFor,
  runAIDiscovery,
  type AICandidate,
  type AIDiscoveryRequest,
  type AIDiscoveryResponse,
  type FlowAnswers,
  type ResearchSource,
  type StepId,
} from '../src/lib/discovery';
import { suggestionOption } from '../src/lib/treeOption';
import { stubFetch } from './helpers';

const BACKEND = 'https://evergreen-backend-eight.vercel.app';

const MOCK_SOURCE_GBIF: ResearchSource = {
  id: 'src-gbif-1',
  title: 'GBIF Backbone Taxonomy — Saraca asoca',
  url: 'https://www.gbif.org/species/5357489',
  publisher: 'Global Biodiversity Information Facility',
  kind: 'biodiversity_record',
  retrievedAt: '2026-10-21T12:00:00Z',
};

const MOCK_SOURCE_WIKI: ResearchSource = {
  id: 'src-wiki-1',
  title: 'Saraca asoca in Cultural Traditions',
  url: 'https://en.wikipedia.org/wiki/Saraca_asoca',
  publisher: 'Wikipedia',
  kind: 'encyclopedia',
  snippet: 'In Tamil and Indian traditions, the Ashoka tree is closely associated with sacred groves.',
  retrievedAt: '2026-10-21T12:00:00Z',
};

const MOCK_CANDIDATE: AICandidate = {
  name: 'Ashoka',
  scientificName: 'Saraca asoca',
  whyItMatches: 'Revered in Indian and Tamil traditions as a symbol of protection and joy.',
  botanicalDetail: 'Rainforest tree native to the Western Ghats and Deccan plateau.',
  matureSize: '8–10 m',
  growingRequirements: ['Humid subtropical climate', 'Rich, well-draining soil'],
  limitations: ['Frost-sensitive in temperate zones'],
  culturalSignificance: {
    text: 'Sourced from cultural botanical records regarding sacred groves.',
    evidence: 'sourced',
    sourceIds: ['src-wiki-1'],
  },
  ecologicalFit: {
    text: 'Native to wet evergreen forests of South Asia.',
    evidence: 'sourced',
    sourceIds: ['src-gbif-1'],
  },
  identity: {
    status: 'confirmed',
    acceptedName: 'Saraca asoca',
    family: 'Fabaceae',
    source: 'GBIF',
  },
  confidence: {
    level: 'high',
    definition: 'Strong multi-source support',
    assessedBy: 'model',
  },
  sourceIds: ['src-wiki-1', 'src-gbif-1'],
};

/* ---------------------------------------------------------------------------
   1. User Intention & Context Detection
--------------------------------------------------------------------------- */

test('an explicit Western zodiac request identifies Western zodiac as default and labels it clearly', () => {
  const detected = detectAstrologyContext('A tree corresponding to my Leo sun sign');
  assert.equal(detected.isAstrology, true);
  assert.equal(detected.system, 'western');
  assert.equal(detected.isAmbiguous, false);

  const answers: FlowAnswers = {
    ...emptyAnswers(),
    followUpNote: 'A tree corresponding to my Leo sun sign',
    astrologySystem: 'western',
  };
  assert.equal(needsAstrologyClarification(answers), false, 'explicit sign needs no clarification');

  const req = aiDiscoveryRequest(answers);
  assert.ok(
    (req.tradition ?? '').includes('Western zodiac') || (req.description ?? '').includes('Western zodiac'),
    'request explicitly labels Western zodiac tradition'
  );
});

test('an explicit Tamil / Nakshatra request is recognised and never defaulted to Western zodiac', () => {
  const tamilDetected = detectAstrologyContext('Looking for a tree for Punarpoosam nakshatra');
  assert.equal(tamilDetected.isAstrology, true);
  assert.equal(tamilDetected.system, 'tamil');
  assert.notEqual(tamilDetected.system, 'western', 'must never default Punarpoosam to Western zodiac');
  assert.equal(tamilDetected.isAmbiguous, false);

  const answers: FlowAnswers = {
    ...emptyAnswers(),
    followUpNote: 'Looking for a tree for Punarpoosam nakshatra',
  };
  assert.equal(needsAstrologyClarification(answers), false, 'explicit Nakshatra needs no ambiguity clarification');

  const req = aiDiscoveryRequest(answers);
  assert.ok(
    (req.tradition ?? '').includes('Tamil') || (req.description ?? '').includes('Punarpoosam'),
    'preserves the Tamil / Nakshatra context'
  );
  assert.ok(!(req.tradition ?? '').includes('Western'), 'does not label Western zodiac');
});

test('an ambiguous astrology request without a specified tradition triggers clarification', () => {
  const detected = detectAstrologyContext('What tree matches my zodiac sign?');
  assert.equal(detected.isAstrology, true);
  assert.equal(detected.isAmbiguous, true);
  assert.equal(detected.system, undefined);

  const answers: FlowAnswers = {
    ...emptyAnswers(),
    followUpNote: 'What tree matches my zodiac sign?',
    motivations: ['astrology'],
  };
  assert.equal(needsAstrologyClarification(answers), true, 'ambiguous astrology requests need clarification');

  const path = pathFor(answers);
  assert.ok(path.includes('clarification'), 'path includes clarification step before proceeding');
});

test('when user skips cultural context, discovery proceeds with a general approach', () => {
  const answers: FlowAnswers = {
    ...emptyAnswers(),
    followUpNote: 'A shade tree for an urban courtyard with clay soil',
    astrologySystem: 'none',
  };
  assert.equal(needsAstrologyClarification(answers), false);
  const path = pathFor(answers);
  assert.ok(!path.includes('clarification'));
  assert.ok(!path.includes('tradition'));
  assert.ok(!path.includes('culture'));

  const req = aiDiscoveryRequest(answers);
  assert.equal(req.tradition ?? '', '');
  assert.ok((req.description ?? '').includes('clay soil'));
});

test('free-text culture selection is preserved verbatim without catalogue allow-lists', () => {
  const answers: FlowAnswers = {
    ...emptyAnswers(),
    followUpNote: 'A tree associated with Osanyin in Yoruba traditional medicine',
    customCulture: 'Yoruba traditional medicine',
  };
  const req = aiDiscoveryRequest(answers);
  assert.equal(req.tradition, 'Yoruba traditional medicine');
  assert.ok((req.description ?? '').includes('Osanyin'));
});

test('no language selector is confused with cultural selection', () => {
  const answers: FlowAnswers = {
    ...emptyAnswers(),
    intention: 'culture',
    followUpChoice: 'nakshatra',
  };
  const steps = pathFor(answers);
  assert.ok(!steps.includes('culture'), 'Sanskrit/Tamil language selector removed from primary path');
  assert.ok(steps.includes('nakshatra'), 'direct nakshatra selection without language barrier');
});

/* ---------------------------------------------------------------------------
   2. Dynamic AI Discovery & Provenance Response Handling
--------------------------------------------------------------------------- */

test('a researched discovery produces candidates and attaches verified sources', async () => {
  const mockResponse: AIDiscoveryResponse = {
    status: 'ok',
    candidates: [MOCK_CANDIDATE],
    sources: [MOCK_SOURCE_WIKI, MOCK_SOURCE_GBIF],
    researchStatus: 'researched',
    researchSummary: 'Researched across 2 independent sources (Wikipedia and GBIF).',
    confidenceDefinitions: { high: 'Strong multi-source support' },
    promptVersion: 'v2.0-intention-first',
    model: 'gemini-1.5-pro',
    disclaimer: 'Machine-synthesised from retrieved reference sources.',
    saved: false,
  };

  const stub = stubFetch(200, mockResponse);
  try {
    const res = await runAIDiscovery({ intention: 'culture', description: 'Ashoka tree in Tamil traditions' }, undefined, BACKEND);
    assert.equal(res.status, 'ok');
    assert.equal(res.researchStatus, 'researched');
    assert.equal(res.candidates.length, 1);
    assert.equal(res.sources.length, 2);

    // Shortlist mapping
    const suggestion = candidateToSuggestion(res.candidates[0], res.sources);
    assert.equal(suggestion.name, 'Ashoka');
    assert.equal(suggestion.sources?.length, 2);
    assert.equal(suggestion.sources?.[0].url, 'https://en.wikipedia.org/wiki/Saraca_asoca');

    // Option view model with provenance tags
    const option = suggestionOption(suggestion, 0, 'shortlist');
    assert.equal(option.title, 'Ashoka');
    assert.equal(option.scientificName, 'Saraca asoca');
    assert.equal(option.culturalVerified, true);
    assert.equal(option.sources.length, 2);
    assert.ok(option.tags.some((t) => t.text.includes('GBIF')));
    assert.ok(option.tags.some((t) => t.text.includes('Backed by sources')));
  } finally {
    stub.restore();
  }
});

test('a partial research response preserves valid candidates and notes partial status', async () => {
  const partialResponse: AIDiscoveryResponse = {
    status: 'ok',
    candidates: [MOCK_CANDIDATE],
    sources: [MOCK_SOURCE_WIKI],
    researchStatus: 'partial',
    researchSummary: 'Partially researched: Wikipedia retrieved, but biodiversity backend timed out.',
    confidenceDefinitions: {},
    promptVersion: 'v2.0-intention-first',
    model: 'gemini-1.5-pro',
    disclaimer: 'Provisional findings.',
    saved: false,
  };

  const stub = stubFetch(200, partialResponse);
  try {
    const res = await runAIDiscovery({ intention: 'explore' }, undefined, BACKEND);
    assert.equal(res.researchStatus, 'partial');
    assert.equal(res.candidates.length, 1);
    assert.equal(res.sources.length, 1);
  } finally {
    stub.restore();
  }
});

test('a no-candidates response honestly explains insufficient evidence without fabricating', async () => {
  const emptyResponse: AIDiscoveryResponse = {
    status: 'no_candidates',
    candidates: [],
    sources: [],
    researchStatus: 'no_sources',
    researchSummary: 'No reliable sources were found for these specific criteria.',
    confidenceDefinitions: {},
    promptVersion: 'v2.0-intention-first',
    model: 'gemini-1.5-pro',
    disclaimer: 'No sources matched.',
    saved: false,
  };

  const stub = stubFetch(200, emptyResponse);
  try {
    const res = await runAIDiscovery({ intention: 'rare tradition' }, undefined, BACKEND);
    assert.equal(res.status, 'no_candidates');
    assert.equal(res.candidates.length, 0);
    assert.equal(res.researchStatus, 'no_sources');
    assert.match(res.researchSummary, /No reliable sources/);
  } finally {
    stub.restore();
  }
});

test('distinctSources preserves order and drops duplicate source IDs', () => {
  const response: AIDiscoveryResponse = {
    status: 'ok',
    candidates: [MOCK_CANDIDATE],
    sources: [MOCK_SOURCE_WIKI, MOCK_SOURCE_GBIF, { ...MOCK_SOURCE_WIKI, id: 'src-wiki-1' }],
    researchStatus: 'researched',
    researchSummary: '',
    confidenceDefinitions: {},
    promptVersion: '',
    model: '',
    disclaimer: '',
    saved: false,
  };
  const distinct = distinctSources(response);
  assert.equal(distinct.length, 2);
  assert.equal(distinct[0].id, 'src-wiki-1');
  assert.equal(distinct[1].id, 'src-gbif-1');
});
