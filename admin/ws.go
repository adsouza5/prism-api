package admin

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type TrafficEvent struct {
	Timestamp string `json:"timestamp"`
	ClientID  string `json:"client_id"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
}

type Metrics struct {
	TotalRequests   int64   `json:"total_requests"`
	AuthedRequests  int64   `json:"authed_requests"`
	RateLimited     int64   `json:"rate_limited"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clients   = map[*websocket.Conn]bool{}
	clientsMu sync.Mutex
	metrics   = &Metrics{}
	metricsMu sync.Mutex
	latencies []int64
)

func Broadcast(evt TrafficEvent) {
	metricsMu.Lock()
	metrics.TotalRequests++
	if evt.Status != 401 {
		metrics.AuthedRequests++
	}
	if evt.Status == 429 {
		metrics.RateLimited++
	}
	latencies = append(latencies, evt.LatencyMs)
	var sum int64
	for _, l := range latencies {
		sum += l
	}
	metrics.AvgLatencyMs = float64(sum) / float64(len(latencies))
	metricsMu.Unlock()

	payload := map[string]interface{}{
		"event":   evt,
		"metrics": metrics,
	}
	data, _ := json.Marshal(payload)

	clientsMu.Lock()
	defer clientsMu.Unlock()
	for conn := range clients {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

func Handler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	// Send current metrics immediately on connect
	metricsMu.Lock()
	data, _ := json.Marshal(map[string]interface{}{"metrics": metrics})
	metricsMu.Unlock()
	conn.WriteMessage(websocket.TextMessage, data)

	// Keep alive — remove on disconnect
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			clientsMu.Lock()
			delete(clients, conn)
			clientsMu.Unlock()
			conn.Close()
			return
		}
	}
}

func GetMetrics(w http.ResponseWriter, r *http.Request) {
	metricsMu.Lock()
	defer metricsMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func timestamp() string {
	return time.Now().Format("15:04:05")
}

var _ = timestamp
