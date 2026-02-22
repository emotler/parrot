package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReplayHandlerMethodNotAllowed(t *testing.T) {
	store := NewStore(10)
	handler := replayHandler(store, 10*time.Second)

	req := httptest.NewRequest("GET", "/_parrot/replay", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp.Error, "POST") {
		t.Errorf("expected error about POST, got: %s", resp.Error)
	}
}

func TestReplayHandlerInvalidJSON(t *testing.T) {
	store := NewStore(10)
	handler := replayHandler(store, 10*time.Second)

	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp.Error, "invalid request body") {
		t.Errorf("expected error about invalid body, got: %s", resp.Error)
	}
}

func TestReplayHandlerMissingID(t *testing.T) {
	store := NewStore(10)
	handler := replayHandler(store, 10*time.Second)

	body := `{"target": "http://example.com"}`
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(body))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "id is required" {
		t.Errorf("expected 'id is required', got: %s", resp.Error)
	}
}

func TestReplayHandlerMissingTarget(t *testing.T) {
	store := NewStore(10)
	handler := replayHandler(store, 10*time.Second)

	body := `{"id": "test-id"}`
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(body))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "target is required" {
		t.Errorf("expected 'target is required', got: %s", resp.Error)
	}
}

func TestReplayHandlerIDNotFound(t *testing.T) {
	store := NewStore(10)
	handler := replayHandler(store, 10*time.Second)

	body := `{"id": "nonexistent", "target": "http://example.com"}`
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(body))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp.Error, "no request found") {
		t.Errorf("expected 'no request found' error, got: %s", resp.Error)
	}
}

func TestReplayHandlerSuccess(t *testing.T) {
	store := NewStore(10)

	// Add a captured request
	capturedReq := EchoResponse{
		ID:        "test-replay-id",
		Timestamp: time.Now(),
		Port:      8080,
		Method:    "POST",
		Path:      "/webhook",
		Headers: map[string]string{
			"Content-Type": "application/json",
			"X-Custom":     "value",
		},
		Body: `{"event":"test"}`,
	}
	store.Add(8080, capturedReq)

	// Create a test server to receive the replay
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify replay headers
		if r.Header.Get("X-Parrot-Replay") != "true" {
			t.Error("expected X-Parrot-Replay header")
		}
		if r.Header.Get("X-Parrot-Replay-ID") != "test-replay-id" {
			t.Error("expected X-Parrot-Replay-ID header")
		}
		if r.Header.Get("X-Custom") != "value" {
			t.Error("expected X-Custom header to be forwarded")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"received"}`))
	}))
	defer targetServer.Close()

	handler := replayHandler(store, 10*time.Second)

	replayReq := ReplayRequest{
		ID:     "test-replay-id",
		Target: targetServer.URL,
	}
	body, _ := json.Marshal(replayReq)
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.OK {
		t.Errorf("expected OK=true, got false with error: %s", resp.Error)
	}
	if resp.ReplayedID != "test-replay-id" {
		t.Errorf("expected ReplayedID test-replay-id, got %s", resp.ReplayedID)
	}
	if resp.Method != "POST" {
		t.Errorf("expected Method POST, got %s", resp.Method)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected StatusCode 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.ResponseBody, "received") {
		t.Errorf("expected response body to contain 'received', got: %s", resp.ResponseBody)
	}
}

func TestReplayHandlerStripHeaders(t *testing.T) {
	store := NewStore(10)

	capturedReq := EchoResponse{
		ID:     "test-strip",
		Method: "POST",
		Headers: map[string]string{
			"Content-Type":      "application/json",
			"X-Keep":            "keep-this",
			"X-Strip":           "strip-this",
			"Host":              "original-host",
			"Content-Length":    "123",
			"Transfer-Encoding": "chunked",
		},
		Body: `{"test":true}`,
	}
	store.Add(8080, capturedReq)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify stripped headers are not present
		if r.Header.Get("X-Strip") != "" {
			t.Error("X-Strip should have been stripped")
		}
		// Verify kept headers are present
		if r.Header.Get("X-Keep") != "keep-this" {
			t.Error("X-Keep should have been kept")
		}
		// Hop-by-hop headers should always be stripped
		if r.Header.Get("Transfer-Encoding") == "chunked" {
			t.Error("Transfer-Encoding should be stripped")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	handler := replayHandler(store, 10*time.Second)

	replayReq := ReplayRequest{
		ID:           "test-strip",
		Target:       targetServer.URL,
		StripHeaders: []string{"X-Strip"},
	}
	body, _ := json.Marshal(replayReq)
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	handler(w, req)

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.OK {
		t.Errorf("expected OK=true, got error: %s", resp.Error)
	}

	// Check that stripped headers are reported
	found := false
	for _, h := range resp.StrippedHeaders {
		if h == "X-Strip" {
			found = true
			break
		}
	}
	if !found {
		t.Error("X-Strip should be in StrippedHeaders list")
	}
}

func TestReplayHandlerStripHeadersCaseInsensitive(t *testing.T) {
	store := NewStore(10)

	capturedReq := EchoResponse{
		ID:     "test-case",
		Method: "GET",
		Headers: map[string]string{
			"X-Custom-Header": "value",
		},
	}
	store.Add(8080, capturedReq)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "" {
			t.Error("X-Custom-Header should have been stripped (case insensitive)")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	handler := replayHandler(store, 10*time.Second)

	// Strip with different case
	replayReq := ReplayRequest{
		ID:           "test-case",
		Target:       targetServer.URL,
		StripHeaders: []string{"x-custom-header"},
	}
	body, _ := json.Marshal(replayReq)
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	handler(w, req)

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.OK {
		t.Errorf("expected OK=true, got error: %s", resp.Error)
	}
}

func TestReplayHandlerTargetError(t *testing.T) {
	store := NewStore(10)

	capturedReq := EchoResponse{
		ID:     "test-error",
		Method: "GET",
	}
	store.Add(8080, capturedReq)

	handler := replayHandler(store, 100*time.Millisecond)

	// Use invalid target URL
	replayReq := ReplayRequest{
		ID:     "test-error",
		Target: "http://localhost:99999/nonexistent",
	}
	body, _ := json.Marshal(replayReq)
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", w.Code)
	}

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.OK {
		t.Error("expected OK=false for failed request")
	}
	if !strings.Contains(resp.Error, "request failed") {
		t.Errorf("expected 'request failed' error, got: %s", resp.Error)
	}
}

func TestReplayHandlerTimeout(t *testing.T) {
	store := NewStore(10)

	capturedReq := EchoResponse{
		ID:     "test-timeout",
		Method: "GET",
	}
	store.Add(8080, capturedReq)

	// Create a slow server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	// Use very short timeout
	handler := replayHandler(store, 50*time.Millisecond)

	replayReq := ReplayRequest{
		ID:     "test-timeout",
		Target: targetServer.URL,
	}
	body, _ := json.Marshal(replayReq)
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", w.Code)
	}

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.OK {
		t.Error("expected OK=false for timeout")
	}
}

func TestReplayHandlerResponseHeaders(t *testing.T) {
	store := NewStore(10)

	capturedReq := EchoResponse{
		ID:     "test-resp-headers",
		Method: "GET",
	}
	store.Add(8080, capturedReq)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response-Header", "test-value")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer targetServer.Close()

	handler := replayHandler(store, 10*time.Second)

	replayReq := ReplayRequest{
		ID:     "test-resp-headers",
		Target: targetServer.URL,
	}
	body, _ := json.Marshal(replayReq)
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	handler(w, req)

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.OK {
		t.Errorf("expected OK=true, got error: %s", resp.Error)
	}

	if resp.ResponseHeaders["X-Response-Header"] != "test-value" {
		t.Error("expected X-Response-Header in response headers")
	}
	if resp.ResponseHeaders["Content-Type"] != "application/json" {
		t.Error("expected Content-Type in response headers")
	}
}

func TestReplayHandlerDurationTracking(t *testing.T) {
	store := NewStore(10)

	capturedReq := EchoResponse{
		ID:     "test-duration",
		Method: "GET",
	}
	store.Add(8080, capturedReq)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	handler := replayHandler(store, 10*time.Second)

	replayReq := ReplayRequest{
		ID:     "test-duration",
		Target: targetServer.URL,
	}
	body, _ := json.Marshal(replayReq)
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	handler(w, req)

	var resp ReplayResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.OK {
		t.Errorf("expected OK=true, got error: %s", resp.Error)
	}

	// Duration should be at least 10ms
	if resp.DurationMs < 10.0 {
		t.Errorf("expected duration >= 10ms, got %f", resp.DurationMs)
	}
}

func TestReplayHandlerOriginalTimestamp(t *testing.T) {
	store := NewStore(10)

	originalTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	capturedReq := EchoResponse{
		ID:        "test-timestamp",
		Timestamp: originalTime,
		Method:    "GET",
	}
	store.Add(8080, capturedReq)

	var receivedTimestamp string
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTimestamp = r.Header.Get("X-Parrot-Original-Timestamp")
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	handler := replayHandler(store, 10*time.Second)

	replayReq := ReplayRequest{
		ID:     "test-timestamp",
		Target: targetServer.URL,
	}
	body, _ := json.Marshal(replayReq)
	req := httptest.NewRequest("POST", "/_parrot/replay", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	handler(w, req)

	if receivedTimestamp == "" {
		t.Error("expected X-Parrot-Original-Timestamp header")
	}

	// Parse and verify timestamp
	parsedTime, err := time.Parse(time.RFC3339Nano, receivedTimestamp)
	if err != nil {
		t.Errorf("failed to parse timestamp: %v", err)
	}
	if !parsedTime.Equal(originalTime) {
		t.Errorf("timestamp mismatch: expected %v, got %v", originalTime, parsedTime)
	}
}
