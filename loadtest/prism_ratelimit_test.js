// prism_ratelimit_test.js — validates Prism's sliding-window rate limiter.
//
// It bursts 2x the configured limit from a SINGLE identity (same API key => same bucket)
// and checks two things that matter in an interview:
//   1. Correctness: allowed (200) should be ~= the configured limit, not far above it.
//      allowed >> limit  =>  the window is leaking (boundary off-by-one, or per-replica
//      counters that don't share state across instances — a real distributed-systems bug).
//   2. Fail-closed: under contention the limiter should return 429, never 5xx.
//      A 5xx under throttle means it fails OPEN when overloaded — worth writing up honestly.
//
// Uses demo-web client (RateLimit: 60/min) — low enough limit to observe throttling quickly.
//
// Run:
//   RL_LIMIT=60 RL_WINDOW=60 BASE_URL=http://localhost:8080 k6 run prism_ratelimit_test.js

import http from 'k6/http';
import { check, fail } from 'k6';
import { Counter } from 'k6/metrics';

const BASE_URL  = __ENV.BASE_URL  || 'http://localhost:8080';
const API_KEY   = __ENV.API_KEY   || 'key_demo_web_123';  // demo-web: RateLimit = 60/min
const LIMIT     = Number(__ENV.RL_LIMIT  || 60);          // must match config.Clients RateLimit
const WINDOW_S  = Number(__ENV.RL_WINDOW || 60);

const allowed    = new Counter('rl_allowed');
const throttled  = new Counter('rl_throttled');
const unexpected = new Counter('rl_unexpected'); // any non-200/429

export const options = {
  scenarios: {
    burst_over_limit: {
      executor: 'shared-iterations',
      vus: 20,
      iterations: LIMIT * 2,   // fire 2× the limit as fast as possible
      maxDuration: '30s',
    },
  },
  thresholds: {
    rl_throttled:  ['count>0'],   // limiter must actually fire
    rl_unexpected: ['count==0'],  // no 5xx under throttle (fail-open check)
  },
};

// setup() exchanges the demo-web API key for a JWT. All VUs use this same token
// so they share a single rate-limit bucket (clientID = "demo-web").
export function setup() {
  const res = http.post(`${BASE_URL}/auth/token`, null, {
    headers: { 'X-API-Key': API_KEY },
  });
  if (res.status !== 200) {
    fail(`auth failed — got ${res.status}: ${res.body}`);
  }
  return { token: JSON.parse(res.body).token };
}

export default function (data) {
  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${data.token}`,
    },
  };

  const payload = JSON.stringify({
    model: 'stub',
    messages: [{ role: 'user', content: 'rl' }],
    max_tokens: 8,
  });

  const res = http.post(`${BASE_URL}/api/echo`, payload, params);

  if (res.status === 200) allowed.add(1);
  else if (res.status === 429) throttled.add(1);
  else unexpected.add(1);

  check(res, {
    'status is 200 or 429 (no 5xx under throttle)': (r) =>
      r.status === 200 || r.status === 429,
    '429 carries rate-limit headers': (r) =>
      r.status !== 429 ||
      r.headers['Retry-After'] !== undefined ||
      r.headers['X-Ratelimit-Remaining'] !== undefined ||
      r.headers['X-Ratelimit-Limit'] !== undefined,
  });
}

export function handleSummary(data) {
  const a = data.metrics.rl_allowed    ? data.metrics.rl_allowed.values.count    : 0;
  const t = data.metrics.rl_throttled  ? data.metrics.rl_throttled.values.count  : 0;
  const u = data.metrics.rl_unexpected ? data.metrics.rl_unexpected.values.count : 0;

  const report =
`
Rate limiter validation
------------------------
  configured limit : ${LIMIT} per ${WINDOW_S}s
  allowed (200)    : ${a}
  throttled (429)  : ${t}
  unexpected (5xx) : ${u}

Interpretation:
  - allowed should be ~= ${LIMIT} (plus a small burst tolerance).
    If allowed >> ${LIMIT}, the window is leaking -> WRITE THIS UP (great finding).
  - unexpected should be 0. Any 5xx means the limiter fails OPEN under load -> also write it up.
  - In-process counters (this Prism) don't share state across Cloud Run replicas:
    per-replica allowed could be ${LIMIT}, aggregate across N replicas = N × ${LIMIT}.
`;
  return {
    stdout: report,
    'results/ratelimit_summary.json': JSON.stringify(data, null, 2),
  };
}
