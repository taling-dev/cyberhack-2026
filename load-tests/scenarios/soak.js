// scenarios/soak.js
//
// 2-hour stability run at constant ~0.28 ops/sec (1 iter/sec from a 5-VU
// pool). Uses constant-arrival-rate so the offered load doesn't sag if
// the cluster is briefly slow.
//
// Goal: surface leaks. Heap should be flat over 2 hours, pool checkouts
// bounded, no pod restarts.
//
//   bash scripts/run-soak.sh
//
// Or directly:
//   k6 run scenarios/soak.js

import { runPipeline } from '../lib/pipeline.js';
import { setupAuth, seedTokenCache } from '../lib/auth.js';

export const options = {
  scenarios: {
    soak: {
      executor: 'constant-arrival-rate',
      rate: 1,                      // 1 iter / timeUnit
      timeUnit: '1s',               // → 1 iter/sec offered
      duration: '2h',
      preAllocatedVUs: 5,
      maxVUs: 8,                    // generous headroom; mock pipeline ≈ 5s
      tags: { scenario: 'soak', phase: 'steady' },
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{rpc_class:non_ai}': ['p(95)<500'],

    // Over 2 hours we expect ~7,000 successful pipelines. Set a floor of
    // 6,500 so mild dips don't fail the run, but a serious hang does.
    'pipeline_e2e_completed{result:success}': ['count>=6500'],

    // Tail latency must not drift upward. If iteration_duration p99 climbs
    // over the run we'd see it as the threshold trips closer to the end.
    iteration_duration: ['p(99)<15000'],
  },
};

// setup() — pre-warm tokens for all 5 demo users. Runs ONCE before VUs
// start; the returned object is passed as the first arg to default().
export const setup = setupAuth;

export default function (data) {
  seedTokenCache(data);
  runPipeline();
}
