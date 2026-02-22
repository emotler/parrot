package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// newID generates a short random hex ID for each captured request.
func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Config holds per-instance runtime settings.
type Config struct {
	Delay         time.Duration
	StatusCode    int
	JSONLogs      bool
	ReplayTimeout time.Duration
	RateLimit     float64 // requests per second, 0 = unlimited
}

// EchoResponse is the JSON body returned for every echoed request.
type EchoResponse struct {
	ID         string            `json:"id"`
	Timestamp  time.Time         `json:"timestamp"`
	Port       int               `json:"port"`
	TLS        bool              `json:"tls"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Path       string            `json:"path"`
	Query      map[string]string `json:"query,omitempty"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body,omitempty"`
	BodyBytes  int               `json:"body_bytes"`
	RemoteAddr string            `json:"remote_addr"`
	DurationMs float64           `json:"duration_ms"`
	StatusCode int               `json:"status_code"`
}

// buildMux constructs the shared ServeMux used by both HTTP and HTTPS servers.
// storePort is always the HTTP port so history is unified per-instance.
func buildMux(storePort int, listenPort int, isTLS bool, store *Store, cfg Config, knownPorts []int) *http.ServeMux {
	mux := http.NewServeMux()

	// History endpoint — always keyed by the HTTP port
	mux.HandleFunc("/_parrot/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		entries := store.GetHistory(storePort)
		json.NewEncoder(w).Encode(entries)
	})

	// Health endpoint
	mux.HandleFunc("/_parrot/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"port":   listenPort,
			"tls":    isTLS,
			"uptime": store.Uptime().String(),
		})
	})

	// HAR export: ?ports= (empty=this port), ?ports=all, ?ports=8080,8081
	mux.HandleFunc("/_parrot/export.har", harHandler(storePort, store, knownPorts))

	// Webhook replay
	mux.HandleFunc("/_parrot/replay", replayHandler(store, cfg.ReplayTimeout))

	// Clear history for this port (DELETE) or all ports (DELETE ?ports=all)
	mux.HandleFunc("/_parrot/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		portsParam := r.URL.Query().Get("ports")
		targets := resolveExportPorts(portsParam, storePort, knownPorts)
		for _, p := range targets {
			store.Clear(p)
		}
		log.Printf("[parrot:%d] history cleared for ports %v", storePort, targets)
		json.NewEncoder(w).Encode(map[string]any{"cleared": targets})
	})

	// Build token bucket rate limiter if configured
	var (
		rateMu     sync.Mutex
		tokens     float64
		lastRefill time.Time
	)
	if cfg.RateLimit > 0 {
		tokens = cfg.RateLimit
		lastRefill = time.Now()
	}

	// Catch-all echo handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Rate limiting — token bucket
		if cfg.RateLimit > 0 {
			rateMu.Lock()
			now := time.Now()
			elapsed := now.Sub(lastRefill).Seconds()
			tokens += elapsed * cfg.RateLimit
			if tokens > cfg.RateLimit {
				tokens = cfg.RateLimit // cap to burst of 1 second
			}
			lastRefill = now
			if tokens < 1 {
				rateMu.Unlock()
				retryAfter := fmt.Sprintf("%.2f", (1-tokens)/cfg.RateLimit)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", cfg.RateLimit))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", retryAfter)
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]any{
					"error":       "rate limit exceeded",
					"limit":       cfg.RateLimit,
					"retry_after": retryAfter,
				})
				log.Printf("[parrot:%d] rate limited %s %s", storePort, r.Method, r.URL.Path)
				return
			}
			tokens--
			rateMu.Unlock()
		}

		start := time.Now()

		if cfg.Delay > 0 {
			time.Sleep(cfg.Delay)
		}

		bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		r.Body.Close()

		headers := make(map[string]string)
		for k, v := range r.Header {
			headers[k] = strings.Join(v, ", ")
		}

		query := make(map[string]string)
		for k, v := range r.URL.Query() {
			query[k] = strings.Join(v, ", ")
		}

		duration := time.Since(start)

		resp := EchoResponse{
			ID:         newID(),
			Timestamp:  start,
			Port:       listenPort,
			TLS:        isTLS,
			Method:     r.Method,
			URL:        r.URL.String(),
			Path:       r.URL.Path,
			Query:      query,
			Headers:    headers,
			Body:       string(bodyBytes),
			BodyBytes:  len(bodyBytes),
			RemoteAddr: r.RemoteAddr,
			DurationMs: float64(duration.Microseconds()) / 1000.0,
			StatusCode: cfg.StatusCode,
		}

		// Store under the HTTP port so history is unified
		store.Add(storePort, resp)

		scheme := "http"
		if isTLS {
			scheme = "https"
		}

		if cfg.JSONLogs {
			line, _ := json.Marshal(resp)
			log.Println(string(line))
		} else {
			log.Printf("[parrot:%d] %s %s %s from %s — %.2fms",
				storePort, strings.ToUpper(scheme), resp.Method, resp.Path, resp.RemoteAddr, resp.DurationMs)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Parrot-Port", fmt.Sprintf("%d", listenPort))
		w.Header().Set("X-Parrot-TLS", fmt.Sprintf("%t", isTLS))
		w.Header().Set("X-Parrot-Duration-Ms", fmt.Sprintf("%.2f", resp.DurationMs))
		if cfg.RateLimit > 0 {
			rateMu.Lock()
			remaining := int(tokens)
			rateMu.Unlock()
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", cfg.RateLimit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		}
		w.WriteHeader(cfg.StatusCode)
		json.NewEncoder(w).Encode(resp)
	})

	return mux
}

// startParrot starts the plain HTTP server for a given port.
func startParrot(port int, store *Store, cfg Config, knownPorts []int) {
	mux := buildMux(port, port, false, store, cfg, knownPorts)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("[parrot:%d] squawking on http://localhost%s", port, addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("[parrot:%d] error: %v", port, err)
	}
}
