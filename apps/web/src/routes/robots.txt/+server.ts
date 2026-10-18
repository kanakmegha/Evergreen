import type { RequestHandler } from './$types';
import { absoluteUrl } from '$lib/site';

/**
 * Rendered per request so the Sitemap line points at the domain actually
 * serving this file. See sitemap.xml/+server.ts for why this is not prerendered.
 */
export const prerender = false;

export const GET: RequestHandler = ({ url }) => {
  const body = `User-agent: *
Allow: /

# Results hold personal notes and are marked noindex.
Disallow: /results/
# Internal knowledge-review tooling.
Disallow: /review

Sitemap: ${absoluteUrl('/sitemap.xml', url.origin)}
`;

  return new Response(body, {
    headers: { 'content-type': 'text/plain; charset=utf-8' },
  });
};
