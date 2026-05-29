import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter(),
    // CSP is owned by SvelteKit so it can hash/nonce its OWN injected
    // inline hydration script (the `dashboard:13` inline <script> that a
    // hand-rolled `script-src 'self'` header was blocking). `mode: 'auto'`
    // uses hashes for SSR'd inline scripts and a nonce for the dev/SPA
    // start script. Non-script directives mirror the previous policy from
    // hooks.server.ts (which no longer sets script-src/style-src).
    csp: {
      mode: 'auto',
      directives: {
        'default-src': ['self'],
        'script-src': ['self'],
        'style-src': ['self', 'unsafe-inline'],
        'img-src': ['self', 'blob:', 'data:', 'https:'],
        'font-src': ['self', 'data:'],
        'connect-src': ['self'],
        'frame-src': ['self'],
        'frame-ancestors': ['none'],
        'base-uri': ['self'],
        'form-action': ['self']
      }
    }
  }
};

export default config;
