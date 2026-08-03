// stub_upstream.go — a fast, dependency-free stand-in for Prism's real LLM backend.
//
// Purpose: isolate the GATEWAY's overhead from the LLM's latency, and avoid burning
// real API spend during load tests. Point Prism's upstream/route at this while benchmarking.
//
// Every millisecond you measure above STUB_LATENCY is Prism's own overhead
// (auth, rate-limit check, routing, proxying, serialization).
//
// Usage:
//   go run stub_upstream.go                                   # :9090, 40ms fixed latency
//   STUB_ADDR=:9090 STUB_LATENCY=40ms STUB_JITTER=10ms go run stub_upstream.go
//   STUB_STREAM=1 go run stub_upstream.go                     # simulate SSE token streaming
//
// It stays deliberately trivial so the stub is never the bottleneck.
package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"
)

func envDur(key, def string) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		v = def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		d, _ = time.ParseDuration(def)
	}
	return d
}

func main() {
	addr := ":9090"
	if p := os.Getenv("STUB_ADDR"); p != "" {
		addr = p
	}
	base := envDur("STUB_LATENCY", "40ms")
	jitter := envDur("STUB_JITTER", "0ms")
	stream := os.Getenv("STUB_STREAM") == "1"

	handler := func(w http.ResponseWriter, r *http.Request) {
		// Simulate upstream think-time so gateway overhead is measurable as the delta.
		d := base
		if jitter > 0 {
			d += time.Duration(rand.Int63n(int64(jitter)))
		}
		time.Sleep(d)

		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming unsupported", http.StatusInternalServerError)
				return
			}
			for i := 0; i < 8; i++ {
				fmt.Fprintf(w, "data: {\"token\":\"t%d\"}\n\n", i)
				fl.Flush()
				time.Sleep(5 * time.Millisecond)
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
			fl.Flush()
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":"stub","object":"chat.completion",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
	// simple health endpoint for warmup / readiness checks
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	fmt.Printf("stub upstream listening on %s (latency=%s jitter=%s stream=%v)\n",
		addr, base, jitter, stream)
	if err := srv.ListenAndServe(); err != nil {
		panic(err)
	}
}
