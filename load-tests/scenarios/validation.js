// scenarios/validation.js
//
// HPA validation — three back-to-back scenarios so each phase carries its
// own `phase=…` tag. k6 doesn't support per-stage tags inside a single
// ramping-vus scenario; stacked scenarios are the idiomatic workaround.
//
// Total wall time ≈ 22 min:
//   * rampup    [0, 1m)    0 → 20 VUs
//   * steady    [1m, 21m)  20 VUs constant
//   * rampdown  [21m, 22m) 20 → 0 VUs
//
// The runner script holds for an additional 10 min after k6 exits so HPA
// scale-down (5-min default stabilization) can fire before postflight.
//
//   bash scripts/run-validation.sh

import { runPipeline } from '../lib/pipeline.js';
import { setupAuth, seedTokenCache } from '../lib/auth.js';
import { sleep } from 'k6';

export const options = {
  scenarios: {
    rampup: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [{ duration: '1m', target: 20 }],
      gracefulRampDown: '0s',
      tags: { scenario: 'validation', phase: 'rampup' },
      exec: 'iter',
    },
    steady: {
      executor: 'constant-vus',
      vus: 20,
      duration: '20m',
      startTime: '1m',
      tags: { scenario: 'validation', phase: 'steady' },
      exec: 'iter',
    },
    rampdown: {
      executor: 'ramping-vus',
      startVUs: 20,
      stages: [{ duration: '1m', target: 0 }],
      gracefulRampDown: '30s',
      startTime: '21m',
      tags: { scenario: 'validation', phase: 'rampdown' },
      exec: 'iter',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{rpc_class:non_ai}': ['p(95)<500'],
    'http_req_duration{rpc:create_lot}': ['p(95)<500'],
    'http_req_duration{rpc:assign_slot}': ['p(95)<800'],
    'pipeline_e2e_completed{result:success}': ['count>=200'],
    iteration_duration: ['p(99)<20000'],
  },
};

export function iter(data) {
  // Idempotent — only the first call per VU populates the token cache
  // from setup()'s pre-warmed tokens. Eliminates the cold-start login
  // storm that triggered Keycloak brute-force lockouts during the
  // 20-VU ramp-up phase of the previous validation run.
  seedTokenCache(data);
  runPipeline();
  // Slight jitter so 20 VUs don't fire simultaneously each iteration.
  sleep(0.1 + Math.random() * 0.2);
}

// setup() — pre-warm tokens for all 5 demo users. Runs ONCE before VUs
// start; k6 passes the returned object as the first arg to every exec
// function (here, `iter`).
export const setup = setupAuth;
