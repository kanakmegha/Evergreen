/**
 * Test stand-in for SvelteKit's `$env/static/public`.
 *
 * The real module is generated per build and only exports the `PUBLIC_`
 * variables that existed when the app was built. `__TEST_*__` identifiers are
 * replaced by `esbuild --define` in `test/run-tests.mjs` (see
 * `test/globals.d.ts`), so the same stub models both "the variable was set" and
 * "the variable was absent".
 */
export const PUBLIC_API_URL = __TEST_PUBLIC_API_URL__;
export const PUBLIC_API_SAME_ORIGIN = __TEST_PUBLIC_SAME_ORIGIN__;
