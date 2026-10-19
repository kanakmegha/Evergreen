/**
 * The configured half of the API base.
 *
 * `test/run-tests.mjs` compiles this file as a production build with the real
 * backend origin in PUBLIC_API_URL, so these checks prove what the deployed
 * application does: with no argument passed anywhere, all three catalogue calls
 * go to the configured backend origin. They exist because the production
 * regression was not a wrong argument — it was a build that carried no origin at
 * all.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  apiBase,
  apiErrorMessage,
  APIError,
  fetchNakshatras,
  fetchRegions,
  fetchTraditions,
  isAPIConfigured,
  missingAPIConfiguration,
} from '../src/lib/discovery';
import { stubFetch } from './helpers';

const BACKEND = 'https://evergreen-backend-eight.vercel.app';

test('PUBLIC_API_URL set at build time is the base the app uses', () => {
  assert.equal(isAPIConfigured(), true);
  assert.equal(apiBase(), BACKEND);
  assert.equal(missingAPIConfiguration(), null);
});

test('with no argument at all, all three catalogue calls use the configured origin', async () => {
  const stub = stubFetch(200, []);
  try {
    await fetchTraditions();
    await fetchNakshatras();
    await fetchRegions('nakshatra-tree-association');
  } finally {
    stub.restore();
  }
  assert.deepEqual(stub.calls, [
    `${BACKEND}/api/v1/traditions`,
    `${BACKEND}/api/v1/nakshatras`,
    `${BACKEND}/api/v1/traditions/nakshatra-tree-association/regions`,
  ]);
});

test('no catalogue URL is left relative to the frontend origin', async () => {
  const stub = stubFetch(200, []);
  try {
    await fetchTraditions();
    await fetchNakshatras();
    await fetchRegions('nakshatra-tree-association');
  } finally {
    stub.restore();
  }
  for (const url of stub.calls) {
    assert.ok(url.startsWith(`${BACKEND}/api/v1/`), `${url} must be absolute and on the backend`);
  }
});

test('a configured build reports real failures as themselves, never as a configuration problem', () => {
  assert.equal(missingAPIConfiguration(), null);
  assert.match(apiErrorMessage(new APIError('not_found', '', 404)), /not found at the configured address \(404\)/i);
  assert.doesNotMatch(apiErrorMessage(new APIError('unavailable', '', 503)), /no catalogue API address configured/i);
});
