package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/adsouza5/prism-api/admin"
	"github.com/adsouza5/prism-api/config"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	clientID := r.Header.Get("X-Client-ID")
	client, ok := config.Clients[clientID]
	if !ok {
		http.Error(w, "unknown client", http.StatusBadRequest)
		return
	}

	target, err := url.Parse(client.UpstreamURL)
	if err != nil {
		http.Error(w, "bad upstream", http.StatusInternalServerError)
		return
	}

	// Strip /api prefix before forwarding to upstream
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}

	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: 200}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Methods")
		resp.Header.Del("Access-Control-Allow-Headers")
		resp.Header.Del("Access-Control-Allow-Credentials")
		return nil
	}
	proxy.ServeHTTP(rec, r)

	latency := time.Since(start).Milliseconds()
	admin.Broadcast(admin.TrafficEvent{
		Timestamp: time.Now().Format("15:04:05"),
		ClientID:  clientID,
		Method:    r.Method,
		Path:      r.URL.Path,
		Status:    rec.status,
		LatencyMs: latency,
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
