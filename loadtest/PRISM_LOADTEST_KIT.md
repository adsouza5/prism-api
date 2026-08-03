# Prism Load-Test & Benchmark Kit

**Goal:** replace "production-grade" *claims* with *measured numbers* — a published benchmark that a Series A founder reads and thinks "this person has actually put a system under load and can reason about where it breaks." Per the portfolio critique, this is the single highest-ROI move you have.

This kit is built for what Prism actually is: a **Go API gateway** (auth → rate limit → quota → route → proxy to an LLM backend) on GCP. It isolates the gateway's own performance from the LLM's latency, so your numbers are honest and cheap to produce.

Files in this kit:
- `stub_upstream.go` — a fast, dependency-free stand-in for the real LLM backend.
- `prism_loadtest.js` — k6 script: smoke / ramp / soak / spike scenarios.
- `prism_ratelimit_test.js` — k6 script that *validates the sliding-window limiter*.
- `run.sh` — orchestrates a full run and saves results with environment metadata.

---

## 0. Where this fits

This is **Gap 1** of four (Prism benchmark → Lens evals artifact → coding/pairing reps → behavioral story bank). Do this one first: the numbers below become the spine of at least three behavioral stories (the sliding-window decision, "what broke first," the cost line) and the proof the portfolio is currently missing.

---

## 1. The one idea that makes this benchmark credible

Most self-run benchmarks are quietly wrong in two ways. Getting these right is itself the interview signal.

### 1a. Isolate the gateway from the LLM
If you load-test Prism against the **real** LLM backend, you're measuring the LLM's latency and burning real API spend — not measuring *your gateway*. Instead, point Prism's upstream/route at `stub_upstream.go`, a local server that returns a fixed response after a **known, configurable** delay. Now every millisecond you measure above that known delay is **Prism's overhead**: auth, rate-limit check, routing, proxying, serialization.

Report it as: *"At 400 req/s against a 40 ms stub upstream, Prism added **X ms** at p99 over the upstream floor."* That sentence is worth more than any "handles production traffic" tagline.

Then do **one small separate pass** against the real backend (a few hundred requests, not a load test) to state end-to-end latency and confirm the added-overhead number holds.

### 1b. Use an open-model load generator (avoid coordinated omission)
The classic benchmarking bug: you spin up N workers that each send a request, wait for the response, then send the next. When the server slows down, your workers *also* slow down, so you stop sending requests exactly when the system is struggling — and your tail latency looks far better than reality. This is **coordinated omission**.

The fix: send requests at a **fixed arrival rate** regardless of how fast responses come back (an "open model"). In k6 that's the `constant-arrival-rate` / `ramping-arrival-rate` executors — which this kit uses for ramp/soak/spike. If you can say "I used an open-model generator to avoid coordinated omission," you're ahead of most senior candidates on this exact topic.

---

## 2. What to measure (and why averages are a trap)

Report the **distribution**, never just the average. A 50 ms average can hide a 4 s p99 that's failing 1% of users.

| Metric | Why it matters |
|---|---|
| p50 / p90 / p95 / p99 / p99.9 / max latency | The tail is the user experience. p99.9 and max expose GC pauses and lock contention. |
| Throughput at the knee (req/s) | The point where latency starts climbing non-linearly = your real capacity. |
| Error rate (non-2xx) under load | A system that stays fast by dropping requests isn't fast. |
| Time-to-first-byte (if streaming SSE) | For an LLM gateway, TTFB matters more than total time. |
| Server CPU, RSS memory, goroutine count, GC pause | Where and why it breaks. Capture from the Go runtime / pprof. |
| Rate-limiter memory footprint | Directly ties to your sliding-window-vs-token-bucket story. |
| $/month to run it | Almost nobody publishes cost. Founders viscerally care. |

**Always record the environment** alongside results, or the numbers aren't reproducible: instance size, Cloud Run concurrency + min/max instances (or VM specs), region, Go version, GOMAXPROCS, k6 version, date, and *where the load generator ran* (same region vs your laptop over WAN — state it).

---

## 3. The test matrix

Run these in order. Each proves a different thing.

| # | Scenario | Executor / profile | Duration | What it proves | Pass bar (tune to your target) |
|---|---|---|---|---|---|
| 1 | **Smoke** | 2 VUs, steady | 1 min | It works at all; wiring is correct | 0 errors, sane latency |
| 2 | **Ramp / step** | arrival rate 10→800 req/s in steps | ~10 min | Finds the **knee** = capacity | Identify req/s where p99 crosses your SLO |
| 3 | **Spike** | baseline → 800 req/s for 10 s → drop | ~4 min | Recovery behavior; does it shed load gracefully or fall over | Recovers to baseline latency within seconds; no lingering 5xx |
| 4 | **Soak** | steady ~100 req/s | 45–60 min | Memory leaks, goroutine leaks, GC drift, connection exhaustion | Flat memory + latency over time (no upward creep) |
| 5 | **Rate-limit validation** | burst 2× the limit from one identity | ~30 s | The limiter actually enforces the limit and fails *closed* (429), not open (5xx) | allowed ≈ configured limit; only 200/429, no 5xx |

The **knee** (test 2) and the **soak trend** (test 4) are the two most important results. The knee is your headline capacity number; the soak proves it's real over time, not a 30-second illusion.

---

## 4. How to run it

Prereqs: install k6 (`brew install k6` / see k6.io docs) and Go.

```bash
# 1. Start the stub upstream (isolates gateway overhead, zero LLM spend)
STUB_LATENCY=40ms STUB_JITTER=10ms go run stub_upstream.go   # listens on :9090

# 2. Point Prism's upstream/route at http://localhost:9090 and start Prism (default :8080)

# 3. Configure and run the full suite
export BASE_URL="http://localhost:8080"     # your Prism address
export TARGET_PATH="/v1/chat"               # the proxied route you want to exercise
export API_KEY="your-gateway-key"           # if the gateway requires auth
chmod +x run.sh && ./run.sh
```

Results (per-scenario `.json` + console `.txt` + `env.txt`) land in `results/<timestamp>/`.

Run individual scenarios directly:
```bash
SCENARIO=ramp  k6 run prism_loadtest.js
SCENARIO=soak  SOAK_RATE=150 SOAK_DURATION=60m k6 run prism_loadtest.js
k6 run prism_ratelimit_test.js       # limiter validation with its own summary
```

**Client-bottleneck check:** while the ramp runs, watch the machine running k6. If *its* CPU saturates, you're measuring your laptop, not Prism — move k6 to a VM (ideally same GCP region as Prism to remove WAN noise) and note it. State WAN vs same-region numbers separately if you have both.

**Cold starts (Cloud Run):** decide what you're measuring. For steady-state capacity, set `min-instances >= 1` and warm it first. If you *want* to show cold-start latency, measure it separately and label it — don't let a cold start contaminate the steady-state distribution.

---

## 5. Reading the results

- **Find the knee:** in the ramp output, walk up the steps until p99 stops being flat and starts climbing. The req/s just before that inflection is your honest capacity number. Report it *with* the upstream stub latency so it's interpretable.
- **Gateway overhead:** `p99(measured) − stub_latency ≈ Prism's added p99`. This is your cleanest, most defensible headline.
- **Soak:** plot latency and RSS over the 45–60 min. Flat = healthy. Upward creep in memory = a leak (often an unbounded map in the rate limiter or a connection pool that never releases — great thing to find and fix). Sawtooth memory with stable latency = normal GC.
- **Rate limiter:** `allowed ≈ limit` means it works. `allowed >> limit` means the window is leaking (off-by-one on the boundary, or per-instance counters that don't share state across replicas — a *real* distributed-systems bug and a superb writeup). Any 5xx under throttle means it fails *open* under contention — also worth writing up honestly.

---

## 6. The writeup (this is the actual deliverable)

A benchmark nobody reads changes nothing. Publish ~800–1,200 words. Use this structure — note it mirrors the behavioral shape (**Stakes → Constraint → Decision → Tradeoff → Scar → Result**), so the post *is* your interview story in written form.

> **Title:** _Load-testing Prism: how much does my Go LLM gateway actually cost per request?_
>
> **1. Why (stakes).** I built Prism to learn what Kong/Envoy do under the hood. But my site claimed it was production-ready and I had zero evidence. So I load-tested it. (1 short para.)
>
> **2. Setup (credibility).** Gateway isolated from the LLM via a stub upstream at a fixed 40 ms; open-model load generator to avoid coordinated omission; full environment table (instance, region, Go version, k6 version, date). Show you know *why* each choice removes a source of error.
>
> **3. Results (the numbers).** The distribution table (p50→p99.9), the throughput knee with a chart, gateway overhead over the stub, and the rate-limiter validation result. Charts > prose here.
>
> **4. The decision + tradeoff.** Why sliding-window over token bucket, and what it cost — e.g. the per-identity memory footprint you measured. This is where you show engineering judgment, not just tooling.
>
> **5. The scar (mandatory).** The one thing that broke or surprised you — a memory creep in the soak, the limiter leaking at the window boundary, a p99.9 spike traced to GC. What you found, how you diagnosed it, what you changed, the before/after number. **A benchmark with no failure reads as either shallow or dishonest.** The critique was explicit: the inflation is more damaging than the gap — so the fix is visible honesty.
>
> **6. Cost (the memorable line).** "Prism sustains ~N req/s at p99 < M ms and I run all six services for **$X/month**." Founders remember the person who knows their bill.
>
> **7. What I'd change at 100×.** Two sentences of failure-mode imagination (shared rate-limit state in Redis, horizontal scaling, where the next bottleneck moves). This is exactly the "founding engineers are hired for failure-mode imagination" signal.

Then: link it from the Prism card on iadamdsouza.com, and replace any "production-grade / handles production traffic" copy with the concrete number. The claim becomes true and specific instead of inflated and vague.

---

## 7. Credibility landmines (each one is a rejection risk)

- Reporting only averages → always show p99+.
- Coordinated omission (closed-model VUs looping) → use arrival-rate executors.
- Letting the real LLM dominate the numbers → use the stub; isolate overhead.
- Client is the bottleneck → watch k6's host CPU; move it off your laptop.
- 30-second "soak" → soak means 45–60 min minimum.
- Unstated environment → nobody can trust or reproduce it.
- No failure in the writeup → reads as shallow. The scar is the point.
