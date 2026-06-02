# Prism — API Gateway

> Production-grade API gateway in Go — JWT authentication, per-client sliding-window rate limiting, reverse proxy, and a WebSocket-powered live traffic dashboard. Deployed on Cloud Run.

Live at **[www.iadamdsouza.com](https://www.iadamdsouza.com)**

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
- **Sliding-window rate limiting** — per-client request quotas enforced in-memory
- **Reverse proxy** — strips `/api` prefix and forwards to the configured upstream
- **WebSocket admin hub** — every proxied request broadcasts method, path, status, and latency to all connected dashboard clients in real time
- **REST metrics snapshot** — `GET /admin/metrics` returns point-in-time counters

## Stack

| Layer | Technology |
|---|---|
| Language | Go |
| Auth | golang-jwt/jwt v5 |
| WebSocket | gorilla/websocket |
| Container | Docker (multi-stage, Alpine) |
| Deployment | Google Cloud Run |

## Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/auth/token` | API Key header | Issue a JWT |
| GET | `/api/*` | Bearer JWT | Proxied + rate-limited |
| GET | `/admin/ws` | — | WebSocket traffic stream |
| GET | `/admin/metrics` | — | Metrics snapshot |
| GET | `/health` | — | Health check |

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

## Deployment

```bash
gcloud builds submit --tag gcr.io/<PROJECT>/prism-api
gcloud run deploy prism-api --image gcr.io/<PROJECT>/prism-api --allow-unauthenticated
```

## License

MIT
