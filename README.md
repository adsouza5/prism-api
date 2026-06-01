# Prism — API Gateway with Auth & Rate Limiting

> Production-grade Go API gateway with JWT authentication, per-client sliding-window rate limiting, and a WebSocket-powered live admin dashboard.

## Overview

Prism is a reverse-proxy API gateway written in Go. It authenticates requests with **JWT**, enforces configurable **sliding-window rate limits** per client key, and proxies traffic to any upstream service. A **WebSocket** feed streams live request events to a React admin UI that shows real-time traffic, per-client quota gauges, and a JWT inspector — all deployed on **Cloud Run**.

## Features

- **JWT authentication** — HS256 token validation on every proxied request
- **Sliding-window rate limiting** — per-client key, configurable RPS and burst
- **Reverse proxy core** — route table maps paths to upstream origins
- **WebSocket live feed** — sub-second latency event stream to the admin UI
- **React admin dashboard** — real-time traffic log, quota bar charts, JWT inspector, rate-limit tester
- **Client management** — named API keys with individual quota and role configuration
- **Cloud Run deployment** — stateless, auto-scaling, zero cold-start on warm traffic

## Stack

| Layer | Technology |
|---|---|
| Gateway core | Go (net/http, httputil.ReverseProxy) |
| Auth | JWT (HS256) |
| Rate limiting | Sliding-window counter (in-memory, per-key) |
| Real-time feed | WebSocket (gorilla/websocket) |
| Frontend | React |
| Deployment | Cloud Run, Docker |

## Architecture

```
Client Request
      ↓
JWT Middleware → Rate Limiter → Reverse Proxy → Upstream
      ↓                ↓
  Reject 401     Reject 429
      
WebSocket broadcast ──→ React Admin Dashboard
(every proxied request)
```

## Live Demo

Available at [adamdsouza.com](https://adamdsouza.com) → Prism project card.

## License

MIT
