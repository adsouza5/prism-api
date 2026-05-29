package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/adsouza5/prism-api/config"
)

type window struct {
	mu         sync.Mutex
	timestamps []time.Time
}

var (
	windows   = map[string]*window{}
	windowsMu sync.Mutex
)

func getWindow(clientID string) *window {
	windowsMu.Lock()
	defer windowsMu.Unlock()
	if w, ok := windows[clientID]; ok {
		return w
	}
	w := &window{}
	windows[clientID] = w
	return w
}

func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.Header.Get("X-Client-ID")
		client, ok := config.Clients[clientID]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		win := getWindow(clientID)
		win.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-time.Minute)

		// drop timestamps outside the window
		valid := win.timestamps[:0]
		for _, t := range win.timestamps {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		win.timestamps = valid

		if len(win.timestamps) >= client.RateLimit {
			win.mu.Unlock()
			w.Header().Set("X-RateLimit-Limit", string(rune(client.RateLimit)))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		win.timestamps = append(win.timestamps, now)
		win.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
