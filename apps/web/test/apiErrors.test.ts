/**
 * The catalogue calls are the ones that broke in production: the deployed
 * bundle carried no backend origin, so `/api/v1/traditions`,
 * `/api/v1/nakshatras` and `/api/v1/traditions/{slug}/regions` were requested
 * from the web app's own origin and came back as HTML 404s.
 *
 * This file is compiled by `test/run-tests.mjs` as a production build with
 * PUBLIC_API_URL ABSENT — the state the deployed frontend was actually in. It
 * pins down three things the fix must keep true: every catalogue call goes to
 * the configured backend origin, a build with no address configured sends no
 * request at all and says so, and nothing is invented when the backend cannot
 * answer. `fetch` is stubbed, so no request leaves the process.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  API_CONFIGURATION_ERROR,
  APIError,
  apiErrorMessage,
  apiURL,
  classifyStatus,
  fetchNakshatras,
  fetchRegions,
  fetchTraditions,
  isAPIConfigured,
  missingAPIConfiguration,
  sameOriginAllowed,
} from '../src/lib/discovery';
import { stubFetch } from './helpers';

const BACKEND = 'https://evergreen-backend-eight.vercel.app';

test('all three catalogue calls use the configured backend origin', async () => {
  const stub = stubFetch(200, []);
  try {
    await fetchTraditions(BACKEND);
    await fetchNakshatras(BACKEND);
    await fetchRegions('nakshatra-tree-association', BACKEND);
  } finally {
    stub.restore();
  }
  assert.deepEqual(stub.calls, [
    `${BACKEND}/api/v1/traditions`,
    `${BACKEND}/api/v1/nakshatras`,
    `${BACKEND}/api/v1/traditions/nakshatra-tree-association/regions`,
  ]);
});

test('a trailing slash on the configured origin is not doubled', async () => {
  const stub = stubFetch(200, []);
  try {
    await fetchTraditions(`${BACKEND}/`);
    await fetchRegions('nakshatra-tree-association', `${BACKEND}///`);
  } finally {
    stub.restore();
  }
  assert.deepEqual(stub.calls, [
    `${BACKEND}/api/v1/traditions`,
    `${BACKEND}/api/v1/traditions/nakshatra-tree-association/regions`,
  ]);
});

test('a tradition slug is percent-encoded in the path', async () => {
  const stub = stubFetch(200, []);
  try {
    await fetchRegions('a b/c', BACKEND);
  } finally {
    stub.restore();
  }
  assert.deepEqual(stub.calls, [`${BACKEND}/api/v1/traditions/a%20b%2Fc/regions`]);
});

test('apiURL joins the configured origin and the path', () => {
  assert.equal(apiURL('/api/v1/traditions', BACKEND), `${BACKEND}/api/v1/traditions`);
  assert.equal(apiURL('/api/v1/nakshatras', `${BACKEND}/`), `${BACKEND}/api/v1/nakshatras`);
});

test('HTTP statuses map to the catalogue states, and only these', () => {
  assert.equal(classifyStatus(404), 'not_found');
  assert.equal(classifyStatus(503), 'unavailable');
  assert.equal(classifyStatus(500), 'server');
  assert.equal(classifyStatus(502), 'server');
  assert.equal(classifyStatus(400), 'request');
});

test('an unavailable backend raises an error and returns no catalogue rows', async () => {
  const calls = [
    () => fetchTraditions(BACKEND),
    () => fetchNakshatras(BACKEND),
    () => fetchRegions('nakshatra-tree-association', BACKEND),
  ];
  for (const call of calls) {
    const stub = stubFetch(503, { error: 'unavailable' });
    try {
      await assert.rejects(call(), (error: unknown) => {
        assert.ok(error instanceof APIError, 'a 503 must not resolve to a value');
        assert.equal(error.kind, 'unavailable');
        assert.equal(error.status, 503);
        return true;
      });
    } finally {
      stub.restore();
    }
  }
});

test('a 404 is classified as a missing route, not as unavailable', async () => {
  const stub = stubFetch(404, '<!doctype html>');
  try {
    await assert.rejects(fetchNakshatras(BACKEND), (error: unknown) => {
      assert.ok(error instanceof APIError);
      assert.equal(error.kind, 'not_found');
      assert.equal(error.status, 404);
      return true;
    });
  } finally {
    stub.restore();
  }
});

test('a transport failure is classified as unreachable, with no status', async () => {
  const original = globalThis.fetch;
  globalThis.fetch = (async () => {
    throw new TypeError('Failed to fetch');
  }) as typeof fetch;
  try {
    await assert.rejects(fetchTraditions(BACKEND), (error: unknown) => {
      assert.ok(error instanceof APIError);
      assert.equal(error.kind, 'network');
      assert.equal(error.status, null);
      return true;
    });
  } finally {
    globalThis.fetch = original;
  }
});

test('a production build without PUBLIC_API_URL sends no catalogue request at all', async () => {
  // This is the state the deployed frontend was in: a build that named no
  // backend origin. It must not quietly turn the three catalogue calls into
  // same-origin requests that can only come back as an HTML 404.
  assert.equal(isAPIConfigured(), false);
  assert.equal(missingAPIConfiguration(), API_CONFIGURATION_ERROR);

  const stub = stubFetch(200, []);
  try {
    for (const call of [
      () => fetchTraditions(),
      () => fetchNakshatras(),
      () => fetchRegions('nakshatra-tree-association'),
    ]) {
      await assert.rejects(call(), (error: unknown) => {
        assert.ok(error instanceof APIError);
        assert.equal(error.kind, 'config');
        assert.equal(error.message, API_CONFIGURATION_ERROR);
        return true;
      });
    }
  } finally {
    stub.restore();
  }

  assert.deepEqual(stub.calls, [], 'a misconfigured build must not reach the network');
});

test('a misconfigured build is not described as a missing catalogue', () => {
  const message = apiErrorMessage(new APIError('config', ''));
  assert.equal(message, API_CONFIGURATION_ERROR);
  assert.match(message, /PUBLIC_API_URL/);
  assert.match(message, /PUBLIC_API_SAME_ORIGIN/);
  assert.doesNotMatch(message, /no traditions|no records|empty/i);
});

test('same-origin is the dev default and a production opt-in', () => {
  assert.equal(sameOriginAllowed(true, undefined), true); // vite dev proxies /api
  assert.equal(sameOriginAllowed(false, undefined), false);
  assert.equal(sameOriginAllowed(false, 'true'), true);
  assert.equal(sameOriginAllowed(false, ' true '), true);
  assert.equal(sameOriginAllowed(false, 'TRUE'), true);
  assert.equal(sameOriginAllowed(false, 'false'), false);
  assert.equal(sameOriginAllowed(false, ''), false);
});

test('only a 404 in an unconfigured build is called a configuration problem', () => {
  const message = apiErrorMessage(new APIError('not_found', '', 404));
  assert.match(message, /no catalogue API address configured/i);
  assert.match(message, /PUBLIC_API_URL/);
  // The failure must not be described as an empty or missing catalogue.
  assert.doesNotMatch(message, /no traditions|no records|empty/i);
  // An unreachable API is still unreachable — it must not be blamed on a build
  // setting the visitor cannot check.
  const offline = apiErrorMessage(new APIError('network', '', null));
  assert.match(offline, /could not be reached/i);
  assert.doesNotMatch(offline, /PUBLIC_API_URL/);
});

test('with an origin configured, 404, 503 and transport failures read differently', () => {
  const notFound = apiErrorMessage(new APIError('not_found', '', 404), true);
  const unavailable = apiErrorMessage(new APIError('unavailable', '', 503), true);
  const network = apiErrorMessage(new APIError('network', '', null), true);

  assert.match(notFound, /404/);
  assert.match(notFound, /PUBLIC_API_URL/);
  assert.match(unavailable, /503/);
  assert.match(network, /could not be reached/i);

  assert.notEqual(notFound, unavailable);
  assert.notEqual(unavailable, network);
  // Only the route problem should point at the configuration.
  assert.doesNotMatch(unavailable, /PUBLIC_API_URL/);
  assert.doesNotMatch(network, /PUBLIC_API_URL/);
});

test('an unknown failure value is treated as unreachable rather than swallowed', () => {
  const message = apiErrorMessage(new Error('boom'), true);
  assert.match(message, /could not be reached/i);
  assert.doesNotMatch(message, /no traditions|no records|PUBLIC_API_URL/);
});
