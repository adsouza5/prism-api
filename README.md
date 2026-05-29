# Prism — API Gateway

Production-grade API gateway built in Go. Handles JWT authentication, per-client sliding-window rate limiting, reverse proxying, and streams live traffic metrics to a WebSocket admin dashboard.

**[Live Demo](https://adsouza5.github.io/portfolio-react/projects/prism)**

## Architecture

```
Client Request
    └─▶ JWT Validation  (Authorization: Bearer <token>)
            └─▶ Rate Limiter  (sliding window, per-client)
                    └─▶ Reverse Proxy  ──▶ Upstream Service
                                              │
Admin Dashboard ◀── WebSocket Hub ◀── TrafficEvent broadcast
```

## Features

- **JWT Auth** — POST `/auth/token` with an API key, receive a signed 24h token (HMAC-SHA256)
- **Sliding-window rate limiting** — per-client request quotas enforced in-memory with `sync.Mutex`
- **Reverse proxy** — `httputil.ReverseProxy` strips the `/api` prefix and forwards to the configured upstream
- **WebSocket admin hub** — every proxied request broadcasts method, path, status, and latency to all connected dashboard clients
- **REST metrics** — GET `/admin/metrics` for a point-in-time snapshot

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.22 |
| Auth | `golang-jwt/jwt` v5 |
| WebSocket | `gorilla/websocket` |
| Container | Docker (multi-stage, alpine) |
| Deployment | Google Cloud Run |

## Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/auth/token` | API Key | Issue a JWT |
| GET | `/api/*` | Bearer JWT | Proxied + rate-limited |
| GET | `/admin/ws` | — | WebSocket stream |
| GET | `/admin/metrics` | — | Current metrics snapshot |
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
curl -X POST http://localhost:8080/auth/token -H "X-API-Key: key_demo_web_123"

# Make a proxied request
curl http://localhost:8080/api/anything -H "Authorization: Bearer <token>"
```

## Deployment

```bash
gcloud builds submit --tag <IMAGE>
gcloud run deploy prism-api --image <IMAGE> --allow-unauthenticated
```
