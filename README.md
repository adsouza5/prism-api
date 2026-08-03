# Prism — API Gateway

> Go API gateway: JWT auth → sliding-window rate limiter → reverse proxy → WebSocket live dashboard. Deployed on Cloud Run.

Live at **[www.iadamdsouza.com](https://www.iadamdsouza.com)**

---

## Architecture

```
Client Request
      │
      ▼
  JWT Middleware          POST /auth/token  ─▶  HMAC-SHA256 signed JWT
      │  valid token
      ▼
  Rate Limiter            sliding window per client key (sync.Mutex)
      │  within quota
      ▼
  Reverse Proxy           strips /api prefix  ─▶  upstream service
      │
      └─▶  TrafficEvent  ─▶  WebSocket Hub  ─▶  Admin Dashboard
                                                  (method · path · status · latency)
```

## Features

- **JWT auth** — `POST /auth/token` with an API key returns a signed 24-hour token
- **Sliding-window rate limiting** — per-client request quotas enforced in-memory; no token bucket rounding, exact window semantics
- **Reverse proxy** — strips `/api` prefix and forwards to the configured upstream
- **WebSocket admin hub** — every proxied request broadcasts method, path, status, and latency to all connected dashboard clients in real time
- **REST metrics snapshot** — `GET /admin/metrics` returns point-in-time counters

## Stack

| Layer | Technology |
|---|---|
| Language | Go 1.26 |
| Auth | golang-jwt/jwt v5 |
| WebSocket | gorilla/websocket |
| Container | Docker (multi-stage, Alpine) |
| Deployment | Google Cloud Run |

## Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/auth/token` | X-API-Key header | Issue a JWT |
| POST/GET | `/api/echo` | Bearer JWT | In-process echo, no upstream |
| ANY | `/api/*` | Bearer JWT | Proxied + rate-limited |
| GET | `/admin/ws` | — | WebSocket traffic stream |
| GET | `/admin/metrics` | — | Metrics snapshot |
| GET | `/health` | — | Health check |

---

## Benchmark Results

**Methodology:** Gateway isolated from any LLM upstream via the `/api/echo` endpoint (in-process, ~0ms upstream latency), so every millisecond measured is Prism's own overhead: JWT validation, sliding-window check, handler dispatch, and admin broadcast. Load generated with k6 using open-model arrival-rate executors to avoid coordinated omission. All runs on localhost (k6 and Prism on the same machine — numbers are a conservative floor; same-region GCP would be faster).

**Environment:** Go 1.26.5 · windows/amd64 · k6 v0.55.0 · single instance · 2026-08-03

---

### Smoke — sanity check (2 VUs, 1 min)

| Metric | Value |
|---|---|
| Requests | 81,192 |
| Error rate | **0.00%** |
| Throughput | 1,353 req/s |
| p95 | 1.04ms |
| p99 | 33ms |
| p99.9 | 80ms |

Pass. All 200s. High p99 here reflects the closed-loop VU model (not open-model) and a GC warmup spike at startup — not representative of steady-state.

---

### Rate-Limit Validation — sliding-window correctness (demo-web client: 60 req/min)

| | |
|---|---|
| Configured limit | 60 req / 60s |
| Allowed (200) | **60** |
| Throttled (429) | **60** |
| Unexpected (5xx) | **0** |

The limiter held to exactly the configured limit — not one request over. Every 429 carried correct `X-RateLimit-Limit`, `X-RateLimit-Remaining: 0`, and `Retry-After: 60` headers. No fail-open behavior under concurrent burst.

**Distributed caveat:** the `windows` map is in-process and not shared across Cloud Run replicas. With N instances, each enforces its own quota independently — aggregate allowed = N × 60. Fix: back the limiter with Redis for shared state across replicas.

---

### Ramp — finding the knee (10→800 req/s over 10 min)

| Metric | Value |
|---|---|
| Requests | 138,600 |
| Error rate | **0.00%** |
| p90 | 815µs |
| p95 | 1.16ms |
| **p99** | **2.51ms** |
| p99.9 | 4.99ms |
| max | 24.81ms |
| TTFB p99 | 2.3ms |

**Knee: not found within 800 req/s.** p99 held flat at 2.51ms all the way to the ceiling of the test — no non-linear climb. Prism's capacity on a single instance exceeds 800 req/s for the echo path. To find the true knee, push past 800 req/s from a separate machine so k6 and Prism don't share CPU.

---

### Spike — 40× burst, shedding and recovery (20 → 800 → 20 req/s)

| Stage | Duration | Behavior |
|---|---|---|
| Baseline | 1 min @ 20 req/s | Stable |
| Ramp up | 10 s: 20 → 800 req/s | Absorbed instantly |
| Hold | 1 min @ 800 req/s | No errors, latency unchanged |
| Drop | 10 s: 800 → 20 req/s | Recovered instantly |
| Recovery window | 2 min @ 20 req/s | Back to baseline |

| Metric | Value |
|---|---|
| Requests | 59,800 |
| Error rate | **0.00%** |
| p95 | 1.03ms |
| **p99** | **2.33ms** |
| p99.9 | 4.57ms |
| max | 27.41ms |

A 40× traffic spike was absorbed with zero errors and p99 indistinguishable from baseline. No lingering 5xx after the drop. Prism sheds and recovers cleanly.

---

### Soak — memory and GC stability (100 req/s × 45 min)

| Metric | Value |
|---|---|
| Requests | 270,001 |
| Duration | 45 min |
| Error rate | **0.00%** |
| Throughput | **100.00 req/s** (flat throughout) |
| p50 | 518µs |
| p90 | 1.58ms |
| p95 | 1.87ms |
| **p99** | **3.14ms** |
| p99.9 | 5.84ms |
| max | 25.93ms |
| TTFB p99 | 2.86ms |

Throughput was exactly 100.00 req/s at every 5-minute sample across all 45 minutes — no drift. p99 held at 3.14ms from start to finish with no upward creep. No goroutine leaks, no latency degradation over time.

**The soak validated a bug fix made during this benchmark pass:** the original `admin/ws.go` appended every request's latency to an unbounded `[]int64` slice and recomputed the average with an O(n) sum loop on every request. At 100 req/s over 45 min that slice would have grown to 270,000 entries and the sum loop would have become the dominant cost well before the run ended, producing visible latency creep. Replaced with a rolling `sum + count` pair — O(1) per request, constant memory. The flat soak line is the proof it works.

---

### Gateway overhead summary

All numbers measured against zero-latency echo upstream — every millisecond is Prism's own cost.

| Scenario | req/s | p99 | p99.9 | Errors |
|---|---|---|---|---|
| Ramp peak (800 req/s) | 800 | 2.51ms | 4.99ms | 0% |
| Spike peak (800 req/s burst) | 800 | 2.33ms | 4.57ms | 0% |
| Soak steady-state (100 req/s, 45 min) | 100 | 3.14ms | 5.84ms | 0% |

**Headline:** Prism sustains 800+ req/s at p99 < 3ms on a single instance with zero errors across smoke, ramp, spike, and a 45-minute soak.

---

### Bugs found and fixed during benchmarking

| File | Bug | Fix |
|---|---|---|
| `middleware/ratelimit.go` | `string(rune(60))` sets `X-RateLimit-Limit` to `"<"` (Unicode code point 60), not `"60"` | `strconv.Itoa(client.RateLimit)` |
| `middleware/ratelimit.go` | 429 responses missing `Retry-After` and `X-RateLimit-Remaining` headers | Added both |
| `admin/ws.go` | Unbounded `[]int64` latency slice + O(n) sum loop on every request — memory creep under soak | Replaced with rolling `sum + count` counters (O(1), constant memory) |

---

## Local Development

```bash
git clone https://github.com/adsouza5/prism-api
cd prism-api
go mod download
go run .
# Server on :8080
```

```bash
# Issue a token
curl -X POST http://localhost:8080/auth/token \
  -H "X-API-Key: key_demo_web_123"

# Make a proxied request
curl http://localhost:8080/api/anything \
  -H "Authorization: Bearer <token>"
```

## Load Testing

```bash
# Install k6: winget install k6  (Windows)  |  brew install k6  (Mac)

# Smoke (sanity — run first)
SCENARIO=smoke BASE_URL=http://localhost:8080 API_KEY=key_loadtest_bench \
  k6 run loadtest/prism_loadtest.js

# Full suite (smoke → ramp → spike → soak → rate-limit validation)
chmod +x loadtest/run.sh && ./loadtest/run.sh

# Rate-limit validation only
RL_LIMIT=60 BASE_URL=http://localhost:8080 k6 run loadtest/prism_ratelimit_test.js
```

See `loadtest/PRISM_LOADTEST_KIT.md` for methodology and result interpretation.

## Deployment

```bash
gcloud builds submit --tag gcr.io/<PROJECT>/prism-api
gcloud run deploy prism-api --image gcr.io/<PROJECT>/prism-api --allow-unauthenticated
```

## License

MIT
