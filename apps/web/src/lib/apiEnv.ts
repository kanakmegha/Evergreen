/**
 * The public environment the API helper depends on, read through SvelteKit's own
 * mechanism.
 *
 * This is the ONLY module that imports SvelteKit's environment modules, so the
 * URL and error logic in `discovery.ts` stays a plain module that can be tested
 * without a bundler.
 *
 * `$env/static/public` is imported as a namespace on purpose. SvelteKit only
 * generates an export for a `PUBLIC_` variable that was actually present when
 * the app was built, so a named import of a variable that was not set would be a
 * missing-named-export failure rather than an absent value. A namespace import
 * makes "not configured" an ordinary `undefined`, which is the state the API
 * helper has to report clearly.
 *
 * SvelteKit's own environment handling does not use Vite's `envPrefix` at all:
 * it filters `process.env` and the `.env` files by the `publicPrefix` from
 * `svelte.config.js`/kit defaults. That is why this module replaced the
 * `import.meta.env` read, which only worked while a hand-written
 * `envPrefix: ['VITE_', 'PUBLIC_']` happened to be present in `vite.config.ts`.
 */
import * as publicEnv from '$env/static/public';
import { dev } from '$app/environment';

const env = publicEnv as unknown as Record<string, string | undefined>;

/** `PUBLIC_API_URL`: the backend origin, or `undefined` when it was not set at build time. */
export const apiURLSetting: string | undefined = env.PUBLIC_API_URL;

/** `PUBLIC_API_SAME_ORIGIN`: the explicit opt-in for a single-origin deployment. */
export const apiSameOriginSetting: string | undefined = env.PUBLIC_API_SAME_ORIGIN;

/** True only under `vite dev`, where the dev server proxies `/api` and `/internal`. */
export const developmentBuild: boolean = dev;
