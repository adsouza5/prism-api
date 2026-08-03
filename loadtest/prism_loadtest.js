// prism_loadtest.js — load test for the Prism Go gateway.
//
// Scenarios (select with SCENARIO=smoke|ramp|soak|spike):
//   smoke : does it work at trivial load; wiring sanity check
//   ramp  : step the arrival rate up to find the KNEE (real capacity)
//   soak  : steady rate for 45m+ to expose memory/goroutine leaks & GC drift
//   spike : sudden burst then drop, to observe shedding & recovery
//
// Ramp/soak/spike use ARRIVAL-RATE executors (open model) on purpose:
// requests are sent at a fixed rate regardless of response time, which avoids
// COORDINATED OMISSION and gives an honest tail. Smoke uses fixed VUs (fine for a sanity check).
//
// Auth: Prism uses a two-step flow.
//   setup() exchanges X-API-Key → JWT via POST /auth/token.
//   The JWT is passed to every VU via the data object.
//
// Run (echo endpoint — no upstream needed, measures pure gateway overhead):
//   SCENARIO=ramp BASE_URL=http://localhost:8080 API_KEY=key_loadtest_bench k6 run prism_loadtest.js
//
// Run (proxy endpoint — start stub first: STUB_LATENCY=40ms go run loadtest/stub_upstream.go):
//   SCENARIO=ramp BASE_URL=http://localhost:8080 TARGET_PATH=/api/anything \
//     PRISM_UPSTREAM_URL=http://localhost:9090 API_KEY=key_loadtest_bench k6 run prism_loadtest.js

import http from 'k6/http';
import { check, fail } from 'k6';
import { Trend } from 'k6/metrics';

const BASE_URL    = __ENV.BASE_URL    || 'http://localhost:8080';
const TARGET_PATH = __ENV.TARGET_PATH || '/api/echo';  // echo: no upstream dep; use /api/* for proxy mode
const API_KEY     = __ENV.API_KEY     || 'key_loadtest_bench';
const SCENARIO    = __ENV.SCENARIO    || 'smoke';

// Time-to-first-byte — the metric that matters most for a streaming LLM gateway.
const ttfb = new Trend('ttfb', true);

const allScenarios = {
  smoke: {
    executor: 'constant-vus',
    vus: 2,
    duration: '1m',
  },
  ramp: {
    executor: 'ramping-arrival-rate',
    startRate: 10,
    timeUnit: '1s',
    preAllocatedVUs: 50,
    maxVUs: 500,
    stages: [
      { target: 50,  duration: '2m' },
      { target: 100, duration: '2m' },
      { target: 200, duration: '2m' },
      { target: 400, duration: '2m' },
      { target: 800, duration: '2m' },
    ],
  },
  soak: {
    executor: 'constant-arrival-rate',
    rate: Number(__ENV.SOAK_RATE || 100),
    timeUnit: '1s',
    duration: __ENV.SOAK_DURATION || '45m',
    preAllocatedVUs: 100,
    maxVUs: 400,
  },
  spike: {
    executor: 'ramping-arrival-rate',
    startRate: 20,
    timeUnit: '1s',
    preAllocatedVUs: 100,
    maxVUs: 1000,
    stages: [
      { target: 20,  duration: '1m'  }, // baseline
      { target: 800, duration: '10s' }, // spike up hard
      { target: 800, duration: '1m'  }, // hold at spike
      { target: 20,  duration: '10s' }, // drop
      { target: 20,  duration: '2m'  }, // observe recovery
    ],
  },
};

export const options = {
  scenarios: { [SCENARIO]: allScenarios[SCENARIO] },
  thresholds: {
    http_req_failed:   ['rate<0.01'],                 // <1% errors
    http_req_duration: ['p(95)<500', 'p(99)<1000'],   // tune to your SLO
    ttfb:              ['p(99)<800'],
  },
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)', 'max'],
};

// setup() runs once before VUs start. Exchanges API key for JWT.
export function setup() {
  const res = http.post(`${BASE_URL}/auth/token`, null, {
    headers: { 'X-API-Key': API_KEY },
  });
  if (res.status !== 200) {
    fail(`auth failed — got ${res.status}: ${res.body}. Is Prism running at ${BASE_URL}?`);
  }
  const { token } = JSON.parse(res.body);
  return { token };
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
    messages: [{ role: 'user', content: 'benchmark ping' }],
    max_tokens: 16,
  });

  const res = http.post(`${BASE_URL}${TARGET_PATH}`, payload, params);

  ttfb.add(res.timings.waiting);

  check(res, {
    'status is 200': (r) => r.status === 200,
    'no error in body': (r) => !String(r.body || '').includes('"error"'),
  });
}
