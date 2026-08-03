package config

import "os"

type Client struct {
	ID          string
	Name        string
	APIKey      string
	RateLimit   int // requests per minute
	UpstreamURL string
}

var Clients map[string]*Client

var JWTSecret = []byte("prism-secret-key-change-in-prod")

func init() {
	// PRISM_UPSTREAM_URL lets you point all proxy clients at a stub for benchmarking:
	//   PRISM_UPSTREAM_URL=http://localhost:9090 go run .
	upstream := os.Getenv("PRISM_UPSTREAM_URL")
	if upstream == "" {
		upstream = "https://httpbin.org"
	}

	Clients = map[string]*Client{
		"demo-web": {
			ID:          "demo-web",
			Name:        "Demo Web App",
			APIKey:      "key_demo_web_123",
			RateLimit:   60,
			UpstreamURL: upstream,
		},
		"mobile-app": {
			ID:          "mobile-app",
			Name:        "Mobile App",
			APIKey:      "key_mobile_456",
			RateLimit:   30,
			UpstreamURL: upstream,
		},
		"data-service": {
			ID:          "data-service",
			Name:        "Data Service",
			APIKey:      "key_data_789",
			RateLimit:   120,
			UpstreamURL: upstream,
		},
		// Effectively unlimited client for load tests — rate limiter must not be the bottleneck.
		// At 2 VUs on localhost the echo endpoint does ~1500 req/s; 1M/min = 16666 req/s headroom.
		"loadtest": {
			ID:          "loadtest",
			Name:        "Load Test Client",
			APIKey:      "key_loadtest_bench",
			RateLimit:   1_000_000,
			UpstreamURL: upstream,
		},
	}
}
