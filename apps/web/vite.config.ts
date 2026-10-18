import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: { proxy: { '/api': 'http://127.0.0.1:8080', '/internal': 'http://127.0.0.1:8080' } },
  // `PUBLIC_` variables are public by contract (SvelteKit documents them as
  // such), and the prefix has to be listed here for `import.meta.env` to see
  // them. Without this, PUBLIC_API_URL read as undefined everywhere and the
  // override silently did nothing. Never put a secret behind PUBLIC_.
  envPrefix: ['VITE_', 'PUBLIC_']
});
