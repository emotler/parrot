package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ReplayRequest is the JSON body POSTed to /_parrot/replay.
type ReplayRequest struct {
	ID           string   `json:"id"`            // ID of the captured request to replay
	Target       string   `json:"target"`        // e.g. "http://localhost:3000/webhook"
	StripHeaders []string `json:"strip_headers"` // header names to drop before forwarding
}

// ReplayResponse is returned to the caller describing what happened.
type ReplayResponse struct {
	OK              bool              `json:"ok"`
	ReplayedID      string            `json:"replayed_id"`
	Target          string            `json:"target"`
	Method          string            `json:"method"`
	StatusCode      int               `json:"status_code,omitempty"`
	DurationMs      float64           `json:"duration_ms"`
	ResponseBody    string            `json:"response_body,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	Error           string            `json:"error,omitempty"`
	StrippedHeaders []string          `json:"stripped_headers,omitempty"`
}

// replayHandler returns an http.HandlerFunc for POST /_parrot/replay.
func replayHandler(store *Store, timeout time.Duration) http.HandlerFunc {
	client := &http.Client{Timeout: timeout}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(ReplayResponse{
				Error: "replay endpoint only accepts POST",
			})
			return
		}

		var req ReplayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ReplayResponse{
				Error: fmt.Sprintf("invalid request body: %v", err),
			})
			return
		}
		r.Body.Close()

		if req.ID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ReplayResponse{Error: "id is required"})
			return
		}
		if req.Target == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ReplayResponse{Error: "target is required"})
			return
		}

		// Look up the original captured request
		entry, ok := store.GetByID(req.ID)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ReplayResponse{
				Error: fmt.Sprintf("no request found with id %q", req.ID),
			})
			return
		}

		// Build the set of headers to strip (case-insensitive)
		stripSet := make(map[string]bool, len(req.StripHeaders))
		for _, h := range req.StripHeaders {
			stripSet[strings.ToLower(h)] = true
		}

		// Always strip hop-by-hop headers that shouldn't be forwarded
		for _, h := range []string{"host", "content-length", "transfer-encoding", "connection"} {
			stripSet[h] = true
		}

		// Build the outbound request
		outReq, err := http.NewRequest(entry.Method, req.Target, strings.NewReader(entry.Body))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ReplayResponse{
				Error: fmt.Sprintf("could not build request: %v", err),
			})
			return
		}

		var stripped []string
		for k, v := range entry.Headers {
			if stripSet[strings.ToLower(k)] {
				stripped = append(stripped, k)
				continue
			}
			outReq.Header.Set(k, v)
		}

		// Tag so the target can identify parrot replays
		outReq.Header.Set("X-Parrot-Replay", "true")
		outReq.Header.Set("X-Parrot-Replay-ID", entry.ID)
		outReq.Header.Set("X-Parrot-Original-Timestamp", entry.Timestamp.UTC().Format(time.RFC3339Nano))

		start := time.Now()
		resp, err := client.Do(outReq)
		durationMs := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(ReplayResponse{
				OK:              false,
				ReplayedID:      req.ID,
				Target:          req.Target,
				Method:          entry.Method,
				DurationMs:      durationMs,
				StrippedHeaders: stripped,
				Error:           fmt.Sprintf("request failed: %v", err),
			})
			return
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB cap

		respHeaders := make(map[string]string, len(resp.Header))
		for k, v := range resp.Header {
			respHeaders[k] = strings.Join(v, ", ")
		}

		json.NewEncoder(w).Encode(ReplayResponse{
			OK:              true,
			ReplayedID:      req.ID,
			Target:          req.Target,
			Method:          entry.Method,
			StatusCode:      resp.StatusCode,
			DurationMs:      durationMs,
			ResponseBody:    string(respBody),
			ResponseHeaders: respHeaders,
			StrippedHeaders: stripped,
		})
	}
}
