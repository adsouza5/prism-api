package proxy

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/adsouza5/prism-api/admin"
)

func EchoHandler(w http.ResponseWriter, r *http.Request) {
	clientID := r.Header.Get("X-Client-ID")
	start := time.Now()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"client":   clientID,
		"path":     r.URL.Path,
		"method":   r.Method,
		"ts":       time.Now().UTC().Format(time.RFC3339),
		"upstream": "echo (internal)",
	})

	admin.Broadcast(admin.TrafficEvent{
		Timestamp: time.Now().Format("15:04:05"),
		ClientID:  clientID,
		Method:    r.Method,
		Path:      "/echo",
		Status:    200,
		LatencyMs: time.Since(start).Milliseconds(),
	})
}
