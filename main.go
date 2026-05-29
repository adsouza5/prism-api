package main

import (
	"log"
	"net/http"
	"time"

	"github.com/adsouza5/prism-api/admin"
	"github.com/adsouza5/prism-api/middleware"
	"github.com/adsouza5/prism-api/proxy"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-API-Key, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func main() {
	mux := http.NewServeMux()

	// Auth endpoint — issue JWT from API key
	mux.HandleFunc("/auth/token", middleware.IssueToken)

	// Admin WebSocket + metrics
	mux.HandleFunc("/admin/ws", admin.Handler)
	mux.HandleFunc("/admin/metrics", admin.GetMetrics)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Proxied routes — require JWT + rate limit
	protected := middleware.ValidateJWT(
		middleware.RateLimit(
			http.HandlerFunc(proxy.Handler),
		),
	)
	mux.Handle("/api/", protected)

	handler := loggingMiddleware(corsMiddleware(mux))

	log.Println("Prism API Gateway running on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}
