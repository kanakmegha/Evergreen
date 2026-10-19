import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: { proxy: { '/api': 'http://127.0.0.1:8080', '/internal': 'http://127.0.0.1:8080' } }

  // `PUBLIC_` variables are read through SvelteKit's `$env/static/public`, which
  // filters the environment by the `PUBLIC_` prefix itself and needs no help
  // from Vite. Vite's `envPrefix` is deliberately NOT set here: it was the only
  // reason `import.meta.env.PUBLIC_API_URL` ever resolved, and removing or
  // reordering it silently dropped the backend origin from the client bundle,
  // which reappeared only as 404s in the browser. Never put a secret behind
  // `PUBLIC_`.
});
