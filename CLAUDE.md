# Prism API Gateway — CLAUDE.md

Go API gateway: auth → rate limit → route → proxy to LLM upstream. No frameworks; stdlib net/http only.

## Key files
- `main.go` — router wiring; all routes registered here
- `middleware/auth.go` — two-step auth: `POST /auth/token` (X-API-Key → JWT), then `ValidateJWT` middleware
- `middleware/ratelimit.go` — in-process sliding-window limiter keyed by clientID; per-instance (not shared across replicas)
- `config/clients.go` — client registry; `PRISM_UPSTREAM_URL` env var overrides all upstreams at startup
- `proxy/router.go` — strips `/api` prefix, proxies to client's UpstreamURL
- `proxy/echo.go` — internal echo endpoint, no upstream dependency
- `admin/ws.go` — WebSocket broadcast + `/admin/metrics` REST endpoint

## Routes
| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/auth/token` | X-API-Key header | Exchange API key for JWT |
| GET | `/health` | none | Health check |
| POST/GET | `/api/echo` | Bearer JWT | Internal echo, no upstream |
| ANY | `/api/*` | Bearer JWT | Reverse proxy; strips `/api` prefix |
| GET | `/admin/metrics` | none | Aggregate metrics JSON |
| WS | `/admin/ws` | none | Live traffic stream |

## Auth flow
```
POST /auth/token
  Header: X-API-Key: key_demo_web_123
  → { "token": "<JWT>", "client_id": "demo-web" }

POST /api/echo
  Header: Authorization: Bearer <JWT>
```

## Clients (config/clients.go)
| ID | API Key | Rate limit |
|----|---------|-----------|
| demo-web | key_demo_web_123 | 60 req/min |
| mobile-app | key_mobile_456 | 30 req/min |
| data-service | key_data_789 | 120 req/min |
| loadtest | key_loadtest_bench | 5000 req/min |

## Running locally
```bash
go run .                                        # gateway on :8080
PRISM_UPSTREAM_URL=http://localhost:9090 go run .   # point all proxy clients at stub
```

## Load testing
```bash
# Install k6: winget install k6 --source winget  (Windows)  |  brew install k6  (Mac)

# Start stub upstream (isolates gateway overhead from real LLM latency)
STUB_LATENCY=40ms STUB_JITTER=10ms go run loadtest/stub_upstream.go   # :9090

# In another terminal — start Prism pointing at stub
PRISM_UPSTREAM_URL=http://localhost:9090 go run .

# Smoke test (sanity check — run this first)
SCENARIO=smoke BASE_URL=http://localhost:8080 API_KEY=key_loadtest_bench \
  k6 run loadtest/prism_loadtest.js

# Full suite
chmod +x loadtest/run.sh && ./loadtest/run.sh

# Rate limiter validation (demo-web: 60 req/min limit)
RL_LIMIT=60 BASE_URL=http://localhost:8080 k6 run loadtest/prism_ratelimit_test.js
```

Results land in `results/<timestamp>/`.

## Known architectural constraints
- **Rate limiter is in-process**: the `windows` map is per-instance. With multiple Cloud Run
  replicas each instance enforces its own 60 req/min — aggregate throughput = 60 × N replicas.
  Fix: back the limiter with Redis for shared state.
- **JWT secret is hardcoded**: `prism-secret-key-change-in-prod` — rotate via env var before any
  real deployment.
