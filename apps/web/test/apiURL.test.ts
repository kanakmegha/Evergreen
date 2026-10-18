/**
 * The API base helper is the single place a backend origin can enter the
 * frontend. A wrong value here is what makes every catalogue request land on
 * the web app instead of the API and return 404, so the joining rules are
 * pinned down rather than left to inspection.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { apiURL, joinAPIBase } from '../src/lib/discovery';

test('a configured API origin is prefixed to the path', () => {
  assert.equal(
    joinAPIBase('https://evergreen-backend-eight.vercel.app', '/api/v1/nakshatras'),
    'https://evergreen-backend-eight.vercel.app/api/v1/nakshatras'
  );
});

test('trailing slashes on the configured origin are not doubled', () => {
  assert.equal(
    joinAPIBase('https://evergreen-backend-eight.vercel.app/', '/api/v1/traditions'),
    'https://evergreen-backend-eight.vercel.app/api/v1/traditions'
  );
  assert.equal(
    joinAPIBase('https://evergreen-backend-eight.vercel.app///', '/api/v1/trees'),
    'https://evergreen-backend-eight.vercel.app/api/v1/trees'
  );
});

test('surrounding whitespace in a configured origin is ignored', () => {
  assert.equal(
    joinAPIBase('  https://backend.example  ', '/api/v1/trees'),
    'https://backend.example/api/v1/trees'
  );
});

test('an unset origin keeps requests relative and same-origin', () => {
  assert.equal(joinAPIBase('', '/api/v1/nakshatras'), '/api/v1/nakshatras');
});

test('a path without a leading slash is still joined correctly', () => {
  assert.equal(joinAPIBase('https://backend.example', 'api/v1/trees'), 'https://backend.example/api/v1/trees');
});

// In the test bundle PUBLIC_API_URL is not defined, so this also proves the
// helper degrades to a same-origin path instead of producing "undefined/...".
test('apiURL falls back to a same-origin path when PUBLIC_API_URL is not set', () => {
  for (const path of ['/api/v1/nakshatras', '/api/v1/traditions', '/api/v1/trees', '/api/v1/questions']) {
    assert.equal(apiURL(path), path);
  }
});
