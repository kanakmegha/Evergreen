/**
 * The single source of truth for Everroot's public web origin.
 *
 * The production domain is deliberately NOT assumed anywhere in the codebase.
 * No host is invented: if `PUBLIC_SITE_URL` is not supplied, absolute URLs fall
 * back to the origin the page is actually being served from, which is by
 * definition correct for whichever domain is in front of the visitor (a
 * platform subdomain, a preview URL, or a real domain).
 *
 * Set `PUBLIC_SITE_URL` (for example `https://everroot.org`) when the canonical
 * origin must differ from the serving host — typically so that a platform
 * subdomain is not indexed as the canonical address.
 *
 * See README.md ("Deploying") for the value that must be supplied before launch.
 */

import { env } from '$env/dynamic/public';

export const SITE_NAME = 'Everroot';
export const SITE_ALTERNATE_NAME = 'Everroot — Knowledge with roots';

/** Social preview image, relative to the site origin. See README: asset required. */
export const OG_IMAGE_PATH = '/og-hero.png';

/**
 * The configured origin without a trailing slash, or an empty string when
 * `PUBLIC_SITE_URL` has not been supplied.
 */
export function configuredOrigin(): string {
  const raw = env.PUBLIC_SITE_URL ?? '';
  return raw.trim().replace(/\/+$/, '');
}

/**
 * Resolve a site path to an absolute URL.
 *
 * `runtimeOrigin` is the origin the current request or page was served from
 * (`page.url.origin`, or `url.origin` inside a `+server.ts` endpoint). It is
 * only consulted when `PUBLIC_SITE_URL` is unset. When neither is available the
 * path is returned relative rather than pointing at an invented host.
 */
export function absoluteUrl(path: string, runtimeOrigin = ''): string {
  const origin = configuredOrigin() || runtimeOrigin.trim().replace(/\/+$/, '');
  const clean = path.startsWith('/') ? path : `/${path}`;
  return origin ? `${origin}${clean}` : clean;
}

/** The public pages search engines may index. Results and review tooling are excluded. */
export const INDEXABLE_PATHS = ['/', '/discover', '/trees'] as const;
