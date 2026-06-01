import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// Browser uploads/downloads QC images directly to/from MinIO via presigned
// URLs, so the MinIO origin must be allowed in connect-src (XHR PUT) and
// img-src (annotated image <img>). Overridable per-deployment; defaults to the
// staging MinIO ingress.
const MINIO_ORIGIN =
  process.env.PUBLIC_MINIO_ORIGIN || 'https://minio.161.118.244.229.sslip.io';

// Dev-only allowance so impeccable live mode can load. Guarded by NODE_ENV.
const __impeccableLiveDev =
  process.env.NODE_ENV === 'development' ? ['http://localhost:8400'] : [];

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
        'script-src': ['self', ...__impeccableLiveDev],
        'style-src': ['self', 'unsafe-inline'],
        'img-src': ['self', 'blob:', 'data:', 'https:'],
        'font-src': ['self', 'data:'],
        'connect-src': ['self', MINIO_ORIGIN, ...__impeccableLiveDev],
        'frame-src': ['self'],
        'frame-ancestors': ['none'],
        'base-uri': ['self'],
        'form-action': ['self']
      }
    }
  }
};

export default config;
