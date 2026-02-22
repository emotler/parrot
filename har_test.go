package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestEchoResponsesToHAR(t *testing.T) {
	entries := []EchoResponse{
		{
			ID:        "test-1",
			Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Port:      8080,
			TLS:       false,
			Method:    "GET",
			URL:       "/api/test?foo=bar",
			Path:      "/api/test",
			Query:     map[string]string{"foo": "bar"},
			Headers: map[string]string{
				"User-Agent":   "test-client",
				"Content-Type": "application/json",
			},
			Body:       "",
			BodyBytes:  0,
			RemoteAddr: "127.0.0.1:12345",
			DurationMs: 15.5,
			StatusCode: 200,
		},
	}

	har := echoResponsesToHAR(entries, "test export")

	if har.Log.Version != "1.2" {
		t.Errorf("expected HAR version 1.2, got %s", har.Log.Version)
	}
	if har.Log.Creator.Name != "parrot" {
		t.Errorf("expected creator name parrot, got %s", har.Log.Creator.Name)
	}
	if har.Log.Comment != "test export" {
		t.Errorf("expected comment 'test export', got %s", har.Log.Comment)
	}
	if len(har.Log.Entries) != 1 {
		t.Fatalf("expected 1 HAR entry, got %d", len(har.Log.Entries))
	}

	entry := har.Log.Entries[0]
	if entry.Request.Method != "GET" {
		t.Errorf("expected method GET, got %s", entry.Request.Method)
	}
	if entry.Request.URL != "http://localhost:8080/api/test?foo=bar" {
		t.Errorf("unexpected URL: %s", entry.Request.URL)
	}
	if entry.Response.Status != 200 {
		t.Errorf("expected status 200, got %d", entry.Response.Status)
	}
	if entry.Time != 15.5 {
		t.Errorf("expected time 15.5, got %f", entry.Time)
	}
}

func TestEchoResponsesToHARWithTLS(t *testing.T) {
	entries := []EchoResponse{
		{
			ID:         "test-tls",
			Timestamp:  time.Now(),
			Port:       9080,
			TLS:        true,
			Method:     "POST",
			URL:        "/webhook",
			Path:       "/webhook",
			Headers:    map[string]string{},
			Body:       `{"event":"test"}`,
			BodyBytes:  16,
			DurationMs: 25.0,
			StatusCode: 201,
		},
	}

	har := echoResponsesToHAR(entries, "")

	entry := har.Log.Entries[0]
	if entry.Request.URL != "https://localhost:9080/webhook" {
		t.Errorf("expected https URL, got %s", entry.Request.URL)
	}
	if entry.Request.PostData == nil {
		t.Fatal("expected PostData to be set")
	}
	if entry.Request.PostData.Text != `{"event":"test"}` {
		t.Errorf("unexpected PostData text: %s", entry.Request.PostData.Text)
	}
	if entry.Request.BodySize != 16 {
		t.Errorf("expected BodySize 16, got %d", entry.Request.BodySize)
	}
}

func TestEchoResponsesToHARSortsChronologically(t *testing.T) {
	entries := []EchoResponse{
		{ID: "third", Timestamp: time.Date(2024, 1, 1, 12, 2, 0, 0, time.UTC), Port: 8080, Method: "GET", URL: "/3"},
		{ID: "first", Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), Port: 8080, Method: "GET", URL: "/1"},
		{ID: "second", Timestamp: time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC), Port: 8080, Method: "GET", URL: "/2"},
	}

	har := echoResponsesToHAR(entries, "")

	if len(har.Log.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(har.Log.Entries))
	}

	// Should be sorted chronologically
	if har.Log.Entries[0].Request.URL != "http://localhost:8080/1" {
		t.Error("first entry should be /1")
	}
	if har.Log.Entries[1].Request.URL != "http://localhost:8080/2" {
		t.Error("second entry should be /2")
	}
	if har.Log.Entries[2].Request.URL != "http://localhost:8080/3" {
		t.Error("third entry should be /3")
	}
}

func TestEchoResponsesToHARHeadersSorted(t *testing.T) {
	entries := []EchoResponse{
		{
			ID:        "test",
			Timestamp: time.Now(),
			Port:      8080,
			Method:    "GET",
			URL:       "/test",
			Headers: map[string]string{
				"Z-Header": "last",
				"A-Header": "first",
				"M-Header": "middle",
			},
		},
	}

	har := echoResponsesToHAR(entries, "")
	headers := har.Log.Entries[0].Request.Headers

	// Headers should be sorted alphabetically
	if len(headers) != 3 {
		t.Fatalf("expected 3 headers, got %d", len(headers))
	}
	if headers[0].Name != "A-Header" {
		t.Errorf("first header should be A-Header, got %s", headers[0].Name)
	}
	if headers[1].Name != "M-Header" {
		t.Errorf("second header should be M-Header, got %s", headers[1].Name)
	}
	if headers[2].Name != "Z-Header" {
		t.Errorf("third header should be Z-Header, got %s", headers[2].Name)
	}
}

func TestResolveExportPorts(t *testing.T) {
	knownPorts := []int{8080, 8081, 8082}

	tests := []struct {
		name       string
		portsParam string
		ownPort    int
		expected   []int
	}{
		{
			name:       "empty defaults to own port",
			portsParam: "",
			ownPort:    8080,
			expected:   []int{8080},
		},
		{
			name:       "all returns all known ports",
			portsParam: "all",
			ownPort:    8080,
			expected:   []int{8080, 8081, 8082},
		},
		{
			name:       "ALL case insensitive",
			portsParam: "ALL",
			ownPort:    8080,
			expected:   []int{8080, 8081, 8082},
		},
		{
			name:       "specific port",
			portsParam: "8081",
			ownPort:    8080,
			expected:   []int{8081},
		},
		{
			name:       "multiple ports",
			portsParam: "8080,8082",
			ownPort:    8081,
			expected:   []int{8080, 8082},
		},
		{
			name:       "ports with spaces",
			portsParam: "8080, 8081, 8082",
			ownPort:    8080,
			expected:   []int{8080, 8081, 8082},
		},
		{
			name:       "invalid port ignored",
			portsParam: "8080,invalid,8081",
			ownPort:    8080,
			expected:   []int{8080, 8081},
		},
		{
			name:       "duplicate ports deduplicated",
			portsParam: "8080,8080,8081",
			ownPort:    8080,
			expected:   []int{8080, 8081},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveExportPorts(tt.portsParam, tt.ownPort, knownPorts)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d ports, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, p := range result {
				if p != tt.expected[i] {
					t.Errorf("at index %d: expected %d, got %d", i, tt.expected[i], p)
				}
			}
		})
	}
}

func TestHARHandler(t *testing.T) {
	store := NewStore(10)
	knownPorts := []int{8080, 8081}

	// Add test data
	store.Add(8080, EchoResponse{
		ID:         "req-8080",
		Timestamp:  time.Now(),
		Port:       8080,
		Method:     "GET",
		URL:        "/test",
		Path:       "/test",
		Headers:    map[string]string{},
		StatusCode: 200,
		DurationMs: 10.0,
	})
	store.Add(8081, EchoResponse{
		ID:         "req-8081",
		Timestamp:  time.Now(),
		Port:       8081,
		Method:     "POST",
		URL:        "/webhook",
		Path:       "/webhook",
		Headers:    map[string]string{},
		StatusCode: 201,
		DurationMs: 15.0,
	})

	handler := harHandler(8080, store, knownPorts)

	tests := []struct {
		name           string
		portsParam     string
		expectedCount  int
		expectedInBody string
	}{
		{
			name:           "default to own port",
			portsParam:     "",
			expectedCount:  1,
			expectedInBody: "req-8080",
		},
		{
			name:           "all ports",
			portsParam:     "all",
			expectedCount:  2,
			expectedInBody: "req-8080",
		},
		{
			name:           "specific port",
			portsParam:     "8081",
			expectedCount:  1,
			expectedInBody: "req-8081",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/_parrot/export.har?ports="+tt.portsParam, nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", contentType)
			}

			var har HAR
			if err := json.NewDecoder(w.Body).Decode(&har); err != nil {
				t.Fatalf("failed to decode HAR: %v", err)
			}

			if len(har.Log.Entries) != tt.expectedCount {
				t.Errorf("expected %d entries, got %d", tt.expectedCount, len(har.Log.Entries))
			}

			body := w.Body.String()
			if tt.expectedInBody != "" && len(body) > 0 {
				// Just verify the response is valid JSON with expected structure
				if har.Log.Version != "1.2" {
					t.Error("invalid HAR version")
				}
			}
		})
	}
}

func TestExportHARToFile(t *testing.T) {
	store := NewStore(10)
	knownPorts := []int{8080}

	store.Add(8080, EchoResponse{
		ID:         "test-export",
		Timestamp:  time.Now(),
		Port:       8080,
		Method:     "GET",
		URL:        "/test",
		Path:       "/test",
		Headers:    map[string]string{"User-Agent": "test"},
		StatusCode: 200,
		DurationMs: 5.0,
	})

	// Create temp file
	tmpfile, err := os.CreateTemp("", "parrot-test-*.har")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	// Export
	err = exportHARToFile(tmpfile.Name(), store, knownPorts)
	if err != nil {
		t.Fatalf("exportHARToFile failed: %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	var har HAR
	if err := json.Unmarshal(data, &har); err != nil {
		t.Fatalf("failed to parse exported HAR: %v", err)
	}

	if har.Log.Version != "1.2" {
		t.Errorf("expected HAR version 1.2, got %s", har.Log.Version)
	}
	if len(har.Log.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(har.Log.Entries))
	}
	if har.Log.Entries[0].Request.Method != "GET" {
		t.Errorf("expected method GET, got %s", har.Log.Entries[0].Request.Method)
	}
}

func TestExportHARToFileEmpty(t *testing.T) {
	store := NewStore(10)
	knownPorts := []int{8080}

	tmpfile, err := os.CreateTemp("", "parrot-test-empty-*.har")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	err = exportHARToFile(tmpfile.Name(), store, knownPorts)
	if err != nil {
		t.Fatalf("exportHARToFile failed: %v", err)
	}

	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	var har HAR
	if err := json.Unmarshal(data, &har); err != nil {
		t.Fatalf("failed to parse exported HAR: %v", err)
	}

	if len(har.Log.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(har.Log.Entries))
	}
}

func TestExportHARToFileInvalidPath(t *testing.T) {
	store := NewStore(10)
	knownPorts := []int{8080}

	err := exportHARToFile("/nonexistent/directory/file.har", store, knownPorts)
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestHARResponseHeaders(t *testing.T) {
	entries := []EchoResponse{
		{
			ID:         "test",
			Timestamp:  time.Now(),
			Port:       8080,
			TLS:        true,
			Method:     "GET",
			URL:        "/test",
			StatusCode: 200,
			DurationMs: 12.34,
		},
	}

	har := echoResponsesToHAR(entries, "")
	respHeaders := har.Log.Entries[0].Response.Headers

	// Check for expected response headers
	headerMap := make(map[string]string)
	for _, h := range respHeaders {
		headerMap[h.Name] = h.Value
	}

	if headerMap["Content-Type"] != "application/json" {
		t.Error("expected Content-Type: application/json")
	}
	if headerMap["X-Parrot-Port"] != "8080" {
		t.Error("expected X-Parrot-Port: 8080")
	}
	if headerMap["X-Parrot-TLS"] != "true" {
		t.Error("expected X-Parrot-TLS: true")
	}
	if headerMap["X-Parrot-Duration-Ms"] != "12.34" {
		t.Errorf("expected X-Parrot-Duration-Ms: 12.34, got %s", headerMap["X-Parrot-Duration-Ms"])
	}
}
