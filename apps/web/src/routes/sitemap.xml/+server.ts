import type { RequestHandler } from './$types';
import { absoluteUrl, INDEXABLE_PATHS } from '$lib/site';

/**
 * Rendered per request rather than prerendered, so the sitemap always matches
 * the domain actually serving it. Prerendering would bake in whichever host the
 * build ran on, which would be wrong for every other environment.
 */
export const prerender = false;

export const GET: RequestHandler = ({ url }) => {
  const entries = INDEXABLE_PATHS.map(
    (path) => `  <url>\n    <loc>${absoluteUrl(path, url.origin)}</loc>\n  </url>`
  ).join('\n');

  const body = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${entries}
</urlset>
`;

  return new Response(body, {
    headers: { 'content-type': 'application/xml; charset=utf-8' },
  });
};
