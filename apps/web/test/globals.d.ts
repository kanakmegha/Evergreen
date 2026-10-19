/**
 * Identifiers that exist only inside the unit-test bundle.
 *
 * `test/run-tests.mjs` builds the test suite twice — once with the API address
 * set at build time and once without — and injects the difference with
 * `esbuild --define`. These declarations give the stubs in `test/stubs/` a type
 * without inventing a runtime value: nothing else in the app may use them.
 */
declare const __TEST_PUBLIC_API_URL__: string | undefined;
declare const __TEST_PUBLIC_SAME_ORIGIN__: string | undefined;
declare const __TEST_APP_DEV__: boolean;
