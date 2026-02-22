package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewID(t *testing.T) {
	id1 := newID()
	id2 := newID()

	if len(id1) != 16 {
		t.Errorf("expected ID length 16, got %d", len(id1))
	}
	if id1 == id2 {
		t.Error("expected unique IDs, got duplicates")
	}
	// Check it's valid hex
	for _, c := range id1 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("expected hex characters, got %c", c)
		}
	}
}

func TestHistoryEndpoint(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	// Add some test data
	store.Add(8080, EchoResponse{
		ID:     "test-1",
		Method: "GET",
		Path:   "/test",
	})
	store.Add(8080, EchoResponse{
		ID:     "test-2",
		Method: "POST",
		Path:   "/api",
	})

	req := httptest.NewRequest("GET", "/_parrot/history", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var history []EchoResponse
	if err := json.NewDecoder(w.Body).Decode(&history); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("expected 2 entries, got %d", len(history))
	}
	if history[0].ID != "test-1" {
		t.Errorf("expected first ID test-1, got %s", history[0].ID)
	}
	if history[1].ID != "test-2" {
		t.Errorf("expected second ID test-2, got %s", history[1].ID)
	}
}

func TestHealthEndpoint(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	req := httptest.NewRequest("GET", "/_parrot/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var health map[string]any
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if health["status"] != "ok" {
		t.Errorf("expected status ok, got %v", health["status"])
	}
	if health["port"] != float64(8080) {
		t.Errorf("expected port 8080, got %v", health["port"])
	}
	if health["tls"] != false {
		t.Errorf("expected tls false, got %v", health["tls"])
	}
	if _, ok := health["uptime"]; !ok {
		t.Error("expected uptime field")
	}
}

func TestHealthEndpointTLS(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8443, true, store, cfg, []int{8080})

	req := httptest.NewRequest("GET", "/_parrot/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var health map[string]any
	json.NewDecoder(w.Body).Decode(&health)

	if health["port"] != float64(8443) {
		t.Errorf("expected port 8443, got %v", health["port"])
	}
	if health["tls"] != true {
		t.Errorf("expected tls true, got %v", health["tls"])
	}
}

func TestClearEndpoint(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080, 8081})

	// Add test data
	store.Add(8080, EchoResponse{ID: "test-1"})
	store.Add(8081, EchoResponse{ID: "test-2"})

	// Clear single port
	req := httptest.NewRequest("DELETE", "/_parrot/clear", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)

	cleared, ok := result["cleared"].([]any)
	if !ok {
		t.Fatal("expected cleared array")
	}
	if len(cleared) != 1 {
		t.Errorf("expected 1 cleared port, got %d", len(cleared))
	}

	// Verify history is cleared for port 8080
	history := store.GetHistory(8080)
	if len(history) != 0 {
		t.Errorf("expected empty history for port 8080, got %d entries", len(history))
	}

	// Verify port 8081 still has data
	history = store.GetHistory(8081)
	if len(history) != 1 {
		t.Errorf("expected 1 entry for port 8081, got %d", len(history))
	}
}

func TestClearEndpointAllPorts(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080, 8081})

	// Add test data
	store.Add(8080, EchoResponse{ID: "test-1"})
	store.Add(8081, EchoResponse{ID: "test-2"})

	// Clear all ports
	req := httptest.NewRequest("DELETE", "/_parrot/clear?ports=all", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify both ports are cleared
	if len(store.GetHistory(8080)) != 0 {
		t.Error("expected empty history for port 8080")
	}
	if len(store.GetHistory(8081)) != 0 {
		t.Error("expected empty history for port 8081")
	}
}

func TestClearEndpointMethodNotAllowed(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	req := httptest.NewRequest("GET", "/_parrot/clear", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestEchoHandler(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	body := `{"test": "data"}`
	req := httptest.NewRequest("POST", "/api/test?foo=bar&baz=qux", strings.NewReader(body))
	req.Header.Set("User-Agent", "test-client")
	req.Header.Set("X-Custom", "value")
	req.RemoteAddr = "127.0.0.1:12345"

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Check custom headers
	if w.Header().Get("X-Parrot-Port") != "8080" {
		t.Errorf("expected X-Parrot-Port 8080, got %s", w.Header().Get("X-Parrot-Port"))
	}
	if w.Header().Get("X-Parrot-TLS") != "false" {
		t.Errorf("expected X-Parrot-TLS false, got %s", w.Header().Get("X-Parrot-TLS"))
	}
	if w.Header().Get("X-Parrot-Duration-Ms") == "" {
		t.Error("expected X-Parrot-Duration-Ms header")
	}

	var resp EchoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Method != "POST" {
		t.Errorf("expected method POST, got %s", resp.Method)
	}
	if resp.Path != "/api/test" {
		t.Errorf("expected path /api/test, got %s", resp.Path)
	}
	if resp.Body != body {
		t.Errorf("expected body %s, got %s", body, resp.Body)
	}
	if resp.BodyBytes != len(body) {
		t.Errorf("expected body_bytes %d, got %d", len(body), resp.BodyBytes)
	}
	if resp.Port != 8080 {
		t.Errorf("expected port 8080, got %d", resp.Port)
	}
	if resp.TLS != false {
		t.Errorf("expected tls false, got %t", resp.TLS)
	}
	if resp.RemoteAddr != "127.0.0.1:12345" {
		t.Errorf("expected remote_addr 127.0.0.1:12345, got %s", resp.RemoteAddr)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status_code 200, got %d", resp.StatusCode)
	}

	// Check query params
	if resp.Query["foo"] != "bar" {
		t.Errorf("expected query foo=bar, got %s", resp.Query["foo"])
	}
	if resp.Query["baz"] != "qux" {
		t.Errorf("expected query baz=qux, got %s", resp.Query["baz"])
	}

	// Check headers
	if resp.Headers["User-Agent"] != "test-client" {
		t.Errorf("expected User-Agent test-client, got %s", resp.Headers["User-Agent"])
	}
	if resp.Headers["X-Custom"] != "value" {
		t.Errorf("expected X-Custom value, got %s", resp.Headers["X-Custom"])
	}

	// Verify stored in history
	history := store.GetHistory(8080)
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}
}

func TestEchoHandlerCustomStatusCode(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 201}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var resp EchoResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.StatusCode != 201 {
		t.Errorf("expected status_code 201, got %d", resp.StatusCode)
	}
}

func TestEchoHandlerWithDelay(t *testing.T) {
	store := NewStore(10)
	cfg := Config{
		StatusCode: 200,
		Delay:      50 * time.Millisecond,
	}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	start := time.Now()
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond {
		t.Errorf("expected delay of at least 50ms, got %v", elapsed)
	}

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestEchoHandlerBodyLimit(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	// Create a body larger than 1MB limit
	largeBody := bytes.Repeat([]byte("x"), 2*1024*1024) // 2MB
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(largeBody))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp EchoResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should be limited to 1MB
	if resp.BodyBytes > 1024*1024 {
		t.Errorf("expected body limited to 1MB, got %d bytes", resp.BodyBytes)
	}
}

func TestEchoHandlerEmptyBody(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp EchoResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Body != "" {
		t.Errorf("expected empty body, got %s", resp.Body)
	}
	if resp.BodyBytes != 0 {
		t.Errorf("expected body_bytes 0, got %d", resp.BodyBytes)
	}
}

func TestRateLimiting(t *testing.T) {
	store := NewStore(10)
	cfg := Config{
		StatusCode: 200,
		RateLimit:  2.0, // 2 requests per second
	}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	// First request should succeed
	req1 := httptest.NewRequest("GET", "/test1", nil)
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)

	if w1.Code != 200 {
		t.Errorf("first request: expected status 200, got %d", w1.Code)
	}
	if w1.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("expected X-RateLimit-Limit 2, got %s", w1.Header().Get("X-RateLimit-Limit"))
	}

	// Second request should succeed
	req2 := httptest.NewRequest("GET", "/test2", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != 200 {
		t.Errorf("second request: expected status 200, got %d", w2.Code)
	}

	// Third request should be rate limited
	req3 := httptest.NewRequest("GET", "/test3", nil)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)

	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("third request: expected status 429, got %d", w3.Code)
	}

	if w3.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("expected X-RateLimit-Limit 2, got %s", w3.Header().Get("X-RateLimit-Limit"))
	}
	if w3.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected X-RateLimit-Remaining 0, got %s", w3.Header().Get("X-RateLimit-Remaining"))
	}
	if w3.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}

	var resp map[string]any
	json.NewDecoder(w3.Body).Decode(&resp)

	if resp["error"] != "rate limit exceeded" {
		t.Errorf("expected error message, got %v", resp["error"])
	}
	if resp["limit"] != 2.0 {
		t.Errorf("expected limit 2, got %v", resp["limit"])
	}
}

func TestRateLimitingRefill(t *testing.T) {
	store := NewStore(10)
	cfg := Config{
		StatusCode: 200,
		RateLimit:  10.0, // 10 requests per second
	}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	// Exhaust the rate limit
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d: expected status 200, got %d", i+1, w.Code)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", w.Code)
	}

	// Wait for tokens to refill (100ms should give us 1 token at 10 req/s)
	time.Sleep(150 * time.Millisecond)

	// Should succeed now
	req2 := httptest.NewRequest("GET", "/test", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Errorf("after refill: expected status 200, got %d", w2.Code)
	}
}

func TestRateLimitingDisabled(t *testing.T) {
	store := NewStore(10)
	cfg := Config{
		StatusCode: 200,
		RateLimit:  0, // disabled
	}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	// Should be able to make many requests without rate limiting
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Errorf("request %d: expected status 200, got %d", i+1, w.Code)
		}
		// Should not have rate limit headers
		if w.Header().Get("X-RateLimit-Limit") != "" {
			t.Error("expected no X-RateLimit-Limit header when rate limiting disabled")
		}
	}
}

func TestEchoHandlerMultiValueHeaders(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Add("Accept", "text/html")
	req.Header.Add("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp EchoResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Multi-value headers should be joined with ", "
	if !strings.Contains(resp.Headers["Accept"], "text/html") {
		t.Error("expected Accept header to contain text/html")
	}
	if !strings.Contains(resp.Headers["Accept"], "application/json") {
		t.Error("expected Accept header to contain application/json")
	}
}

func TestEchoHandlerMultiValueQuery(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	req := httptest.NewRequest("GET", "/test?tag=foo&tag=bar", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp EchoResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Multi-value query params should be joined with ", "
	if !strings.Contains(resp.Query["tag"], "foo") {
		t.Error("expected tag query to contain foo")
	}
	if !strings.Contains(resp.Query["tag"], "bar") {
		t.Error("expected tag query to contain bar")
	}
}

func TestEchoHandlerStoresInCorrectPort(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}

	// Create mux for HTTPS port 8443, but store under HTTP port 8080
	mux := buildMux(8080, 8443, true, store, cfg, []int{8080})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should be stored under port 8080 (storePort), not 8443 (listenPort)
	history := store.GetHistory(8080)
	if len(history) != 1 {
		t.Errorf("expected 1 entry in port 8080 history, got %d", len(history))
	}

	// But the response should show port 8443
	var resp EchoResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Port != 8443 {
		t.Errorf("expected response port 8443, got %d", resp.Port)
	}
	if resp.TLS != true {
		t.Errorf("expected response tls true, got %t", resp.TLS)
	}
}

func TestEchoResponseJSONMarshaling(t *testing.T) {
	resp := EchoResponse{
		ID:         "test-id",
		Timestamp:  time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC),
		Port:       8080,
		TLS:        true,
		Method:     "POST",
		URL:        "/api/test?foo=bar",
		Path:       "/api/test",
		Query:      map[string]string{"foo": "bar"},
		Headers:    map[string]string{"User-Agent": "test"},
		Body:       "test body",
		BodyBytes:  9,
		RemoteAddr: "127.0.0.1:12345",
		DurationMs: 1.23,
		StatusCode: 201,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded EchoResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != resp.ID {
		t.Errorf("ID mismatch: expected %s, got %s", resp.ID, decoded.ID)
	}
	if decoded.Port != resp.Port {
		t.Errorf("Port mismatch: expected %d, got %d", resp.Port, decoded.Port)
	}
	if decoded.TLS != resp.TLS {
		t.Errorf("TLS mismatch: expected %t, got %t", resp.TLS, decoded.TLS)
	}
	if decoded.Method != resp.Method {
		t.Errorf("Method mismatch: expected %s, got %s", resp.Method, decoded.Method)
	}
	if decoded.Body != resp.Body {
		t.Errorf("Body mismatch: expected %s, got %s", resp.Body, decoded.Body)
	}
}

func TestEchoResponseOmitEmpty(t *testing.T) {
	resp := EchoResponse{
		ID:         "test-id",
		Timestamp:  time.Now(),
		Port:       8080,
		Method:     "GET",
		Path:       "/test",
		Headers:    map[string]string{},
		BodyBytes:  0,
		RemoteAddr: "127.0.0.1",
		DurationMs: 1.0,
		StatusCode: 200,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)

	// Query and Body should be omitted when empty
	if strings.Contains(jsonStr, `"query"`) {
		t.Error("expected query field to be omitted when empty")
	}
	if strings.Contains(jsonStr, `"body"`) {
		t.Error("expected body field to be omitted when empty")
	}

	// Other fields should be present
	if !strings.Contains(jsonStr, `"id"`) {
		t.Error("expected id field to be present")
	}
	if !strings.Contains(jsonStr, `"headers"`) {
		t.Error("expected headers field to be present even when empty")
	}
}

func TestBuildMuxWithDifferentPorts(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	knownPorts := []int{8080, 8081, 8082}

	// Test with different store and listen ports
	mux := buildMux(8080, 8443, true, store, cfg, knownPorts)

	if mux == nil {
		t.Fatal("buildMux returned nil")
	}

	// Verify health endpoint returns correct port info
	req := httptest.NewRequest("GET", "/_parrot/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var health map[string]any
	json.NewDecoder(w.Body).Decode(&health)

	if health["port"] != float64(8443) {
		t.Errorf("expected port 8443, got %v", health["port"])
	}
	if health["tls"] != true {
		t.Errorf("expected tls true, got %v", health["tls"])
	}
}

func TestEchoHandlerReadBodyError(t *testing.T) {
	store := NewStore(10)
	cfg := Config{StatusCode: 200}
	mux := buildMux(8080, 8080, false, store, cfg, []int{8080})

	// Create a reader that will error
	errorReader := io.NopCloser(&errorReader{})
	req := httptest.NewRequest("POST", "/test", errorReader)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should still return 200 and handle gracefully
	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp EchoResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Body should be empty or partial
	if resp.BodyBytes > 0 {
		t.Logf("got partial body: %d bytes", resp.BodyBytes)
	}
}

// errorReader is a reader that always returns an error
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}
