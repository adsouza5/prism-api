#!/usr/bin/env bash
set -euo pipefail

# ---- config: adjust to your Prism deployment ----
export BASE_URL="${BASE_URL:-http://localhost:8080}"
export TARGET_PATH="${TARGET_PATH:-/api/echo}"         # echo: no upstream dep; /api/* for proxy mode
export API_KEY="${API_KEY:-key_loadtest_bench}"        # loadtest client: 5000 req/min, won't throttle
export RL_API_KEY="${RL_API_KEY:-key_demo_web_123}"    # demo-web: 60 req/min, for rate-limit validation

# ---- stub upstream (optional, for proxy overhead isolation) ----
# To measure gateway overhead over a known-latency upstream:
#   1. In another terminal: STUB_LATENCY=40ms STUB_JITTER=10ms go run loadtest/stub_upstream.go
#   2. Start Prism with: PRISM_UPSTREAM_URL=http://localhost:9090 go run .
#   3. Set TARGET_PATH=/api/anything (or any path that routes through the proxy handler)
#
# Without stub: TARGET_PATH=/api/echo measures pure in-process gateway overhead (~0ms upstream).

OUT="results/$(date +%Y%m%d_%H%M%S)"
mkdir -p "$OUT"

echo "==> Recording environment (reproducibility)"
{
  echo "date       : $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "BASE_URL   : $BASE_URL"
  echo "TARGET_PATH: $TARGET_PATH"
  echo "host       : $(uname -a)"
  command -v go >/dev/null 2>&1 && echo "go         : $(go version)"
  command -v k6 >/dev/null 2>&1 && echo "k6         : $(k6 version)"
  echo "NOTE: also record Prism instance size, Cloud Run concurrency + min/max instances,"
  echo "      region, GOMAXPROCS, and WHERE this load generator ran (same-region vs WAN)."
} | tee "$OUT/env.txt"

echo "==> 1/5 smoke (sanity — confirms wiring before burning time on longer runs)"
SCENARIO=smoke k6 run --summary-export "$OUT/smoke.json" loadtest/prism_loadtest.js | tee "$OUT/smoke.txt"

echo "==> 2/5 ramp (find the knee = honest capacity)"
SCENARIO=ramp k6 run --summary-export "$OUT/ramp.json" loadtest/prism_loadtest.js | tee "$OUT/ramp.txt"

echo "==> 3/5 spike (shedding + recovery)"
SCENARIO=spike k6 run --summary-export "$OUT/spike.json" loadtest/prism_loadtest.js | tee "$OUT/spike.txt"

echo "==> 4/5 soak (memory/goroutine leaks, GC drift — run 45m minimum)"
SCENARIO=soak SOAK_RATE="${SOAK_RATE:-100}" SOAK_DURATION="${SOAK_DURATION:-45m}" \
  k6 run --summary-export "$OUT/soak.json" loadtest/prism_loadtest.js | tee "$OUT/soak.txt"

echo "==> 5/5 rate limiter validation (demo-web client: limit=60/min)"
RL_LIMIT="${RL_LIMIT:-60}" RL_WINDOW="${RL_WINDOW:-60}" API_KEY="$RL_API_KEY" \
  k6 run --summary-export "$OUT/ratelimit.json" loadtest/prism_ratelimit_test.js | tee "$OUT/ratelimit.txt"

echo "==> done. results in $OUT"
echo "    Next: find the knee in ramp.txt, compute gateway overhead (p99 - stub_latency if using stub),"
echo "    check soak.txt for latency/memory creep, and drop the numbers into the writeup skeleton."
