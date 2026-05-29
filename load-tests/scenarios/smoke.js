// scenarios/smoke.js
//
// Sanity check: 1 VU, 1 minute. Runs the full pipeline as fast as the
// AI worker allows (~5s per pipeline). Tightest thresholds — fails on
// anything weird.
//
//   bash scripts/run-smoke.sh
//
// Or invoke directly:
//   k6 run scenarios/smoke.js

import { runPipeline } from '../lib/pipeline.js';
import { setupAuth, seedTokenCache } from '../lib/auth.js';

export const options = {
  scenarios: {
    smoke: {
      executor: 'constant-vus',
      vus: 1,
      duration: '60s',
      tags: { scenario: 'smoke', phase: 'steady' },
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{rpc_class:non_ai}': ['p(95)<1000'],
    pipeline_e2e_completed: ['count>=5'],
    'pipeline_e2e_completed{result:success}': ['count>=5'],
    iteration_duration: ['p(99)<15000'],
  },
};

// setup() — pre-warm tokens for all 5 demo users. Runs ONCE before VUs
// start; the returned object is passed as the first arg to default().
export const setup = setupAuth;

export default function (data) {
  // Idempotent — only the first call per VU populates the token cache.
  seedTokenCache(data);
  runPipeline();
}
