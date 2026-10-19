/**
 * Run the unit tests against two different builds of the same source.
 *
 * The API base is a build-time value, so "the variable was set" and "the
 * variable was absent" are two different builds, not two different inputs. This
 * runner compiles the suite once per state with `$env/static/public` and
 * `$app/environment` aliased to the stubs in `test/stubs/`, so the tests
 * exercise the real `apiEnv.ts` wiring rather than a hand-passed value.
 *
 * No request leaves the process: the suites stub `fetch` themselves.
 */
import { build } from 'esbuild';
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const web = path.resolve(here, '..');
const out = path.join(web, '.tmp-test');

/** The real production backend origin, so the configured build mirrors production. */
const CONFIGURED_ORIGIN = 'https://evergreen-backend-eight.vercel.app';

const shared = {
  bundle: true,
  platform: 'node',
  format: 'esm',
  logLevel: 'warning',
  alias: {
    '$env/static/public': path.join(here, 'stubs/env-public.ts'),
    '$app/environment': path.join(here, 'stubs/app-environment.ts'),
  },
};

const groups = [
  {
    name: 'production build without PUBLIC_API_URL',
    entries: ['apiURL.test.ts', 'apiErrors.test.ts', 'treeOption.test.ts', 'recommendations.test.ts'],
    define: {
      __TEST_PUBLIC_API_URL__: 'undefined',
      __TEST_PUBLIC_SAME_ORIGIN__: 'undefined',
      __TEST_APP_DEV__: 'false',
    },
  },
  {
    name: 'production build with PUBLIC_API_URL set',
    entries: ['apiConfigured.test.ts', 'recommendationsAPI.test.ts'],
    define: {
      __TEST_PUBLIC_API_URL__: JSON.stringify(CONFIGURED_ORIGIN),
      __TEST_PUBLIC_SAME_ORIGIN__: 'undefined',
      __TEST_APP_DEV__: 'false',
    },
  },
];

fs.rmSync(out, { recursive: true, force: true });

let failed = false;
for (const group of groups) {
  const result = await build({
    ...shared,
    entryPoints: group.entries.map((entry) => path.join(here, entry)),
    outdir: out,
    define: group.define,
  });
  if (result.errors.length > 0) {
    failed = true;
    continue;
  }

  console.log(`\n=== ${group.name} ===`);
  const run = spawnSync(
    process.execPath,
    ['--test', ...group.entries.map((entry) => path.join(out, entry.replace(/\.ts$/, '.js')))],
    { stdio: 'inherit', cwd: web }
  );
  if (run.status !== 0) failed = true;
}

fs.rmSync(out, { recursive: true, force: true });
process.exit(failed ? 1 : 0);
