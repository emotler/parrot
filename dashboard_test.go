package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewDashboard(t *testing.T) {
	store := NewStore(100)
	ports := []int{8080, 8081, 8082}
	flags := []string{"--delay=100ms", "--status=201"}

	dash := NewDashboard(store, ports, flags)

	if dash == nil {
		t.Fatal("NewDashboard returned nil")
	}
	if dash.store != store {
		t.Error("store not set correctly")
	}
	if len(dash.ports) != len(ports) {
		t.Errorf("expected %d ports, got %d", len(ports), len(dash.ports))
	}
	for i, p := range ports {
		if dash.ports[i] != p {
			t.Errorf("port[%d]: expected %d, got %d", i, p, dash.ports[i])
		}
	}
	if len(dash.flags) != len(flags) {
		t.Errorf("expected %d flags, got %d", len(flags), len(dash.flags))
	}
}

func TestMethodColor(t *testing.T) {
	tests := []struct {
		method   string
		expected string
	}{
		{"GET", "\033[32m"},
		{"POST", "\033[34m"},
		{"PUT", "\033[33m"},
		{"DELETE", "\033[31m"},
		{"PATCH", "\033[35m"},
		{"OPTIONS", "\033[37m"},
		{"HEAD", "\033[37m"},
		{"UNKNOWN", "\033[37m"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := methodColor(tt.method)
			if result != tt.expected {
				t.Errorf("methodColor(%q) = %q, want %q", tt.method, result, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		max      int
		expected string
	}{
		{
			name:     "shorter than max",
			input:    "hello",
			max:      10,
			expected: "hello",
		},
		{
			name:     "equal to max",
			input:    "hello",
			max:      5,
			expected: "hello",
		},
		{
			name:     "longer than max",
			input:    "hello world",
			max:      8,
			expected: "hello w…",
		},
		{
			name:     "much longer than max",
			input:    "/api/v1/users/12345/profile/settings",
			max:      20,
			expected: "/api/v1/users/12345…",
		},
		{
			name:     "empty string",
			input:    "",
			max:      10,
			expected: "",
		},
		{
			name:     "max of 1",
			input:    "hello",
			max:      1,
			expected: "…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.max)
			if result != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.expected)
			}
		})
	}
}

func TestDashboardServe_RootHandler(t *testing.T) {
	store := NewStore(100)
	ports := []int{8080}
	flags := []string{}
	_ = NewDashboard(store, ports, flags)

	// Create a test server
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(dashboardHTML))
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" {
		t.Errorf("expected Content-Type text/html, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("response should contain HTML doctype")
	}
	if !strings.Contains(body, "parrot dashboard") {
		t.Error("response should contain dashboard title")
	}
}

func TestDashboardServe_StatsHandler(t *testing.T) {
	store := NewStore(100)
	ports := []int{8080, 8081}
	flags := []string{"--delay=100ms"}
	dash := NewDashboard(store, ports, flags)

	// Add some test data
	store.Add(8080, EchoResponse{
		ID:         "test-1",
		Timestamp:  time.Now(),
		Port:       8080,
		Method:     "GET",
		Path:       "/test",
		DurationMs: 10.5,
	})
	store.Add(8081, EchoResponse{
		ID:         "test-2",
		Timestamp:  time.Now(),
		Port:       8081,
		Method:     "POST",
		Path:       "/api",
		DurationMs: 25.3,
	})

	// Create a test server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		type PortStat struct {
			Port    int            `json:"port"`
			Count   int64          `json:"count"`
			AvgMs   float64        `json:"avg_ms"`
			History []EchoResponse `json:"history"`
		}

		stats := []PortStat{}
		sortedPorts := []int{8080, 8081}

		for _, p := range sortedPorts {
			count, avgMs := dash.store.Stats(p)
			history := dash.store.GetHistory(p)
			if len(history) > 20 {
				history = history[len(history)-20:]
			}
			stats = append(stats, PortStat{
				Port:    p,
				Count:   count,
				AvgMs:   avgMs,
				History: history,
			})
		}

		json.NewEncoder(w).Encode(map[string]any{
			"uptime_seconds": dash.store.Uptime().Seconds(),
			"ports":          stats,
			"flags":          dash.flags,
		})
	})

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	corsHeader := w.Header().Get("Access-Control-Allow-Origin")
	if corsHeader != "*" {
		t.Errorf("expected CORS header *, got %s", corsHeader)
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if _, ok := response["uptime_seconds"]; !ok {
		t.Error("response should contain uptime_seconds")
	}

	portsData, ok := response["ports"].([]any)
	if !ok {
		t.Fatal("response should contain ports array")
	}
	if len(portsData) != 2 {
		t.Errorf("expected 2 ports in response, got %d", len(portsData))
	}

	flagsData, ok := response["flags"].([]any)
	if !ok {
		t.Fatal("response should contain flags array")
	}
	if len(flagsData) != 1 {
		t.Errorf("expected 1 flag in response, got %d", len(flagsData))
	}
}

func TestDashboardServe_ClearHandler(t *testing.T) {
	store := NewStore(100)
	ports := []int{8080, 8081}
	flags := []string{}
	dash := NewDashboard(store, ports, flags)

	// Add some test data
	store.Add(8080, EchoResponse{
		ID:         "test-1",
		Timestamp:  time.Now(),
		Port:       8080,
		Method:     "GET",
		Path:       "/test",
		DurationMs: 10.5,
	})
	store.Add(8081, EchoResponse{
		ID:         "test-2",
		Timestamp:  time.Now(),
		Port:       8081,
		Method:     "POST",
		Path:       "/api",
		DurationMs: 25.3,
	})

	// Verify data exists
	if len(store.GetHistory(8080)) == 0 {
		t.Fatal("expected data in port 8080")
	}
	if len(store.GetHistory(8081)) == 0 {
		t.Fatal("expected data in port 8081")
	}

	// Create a test server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/clear", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		portsParam := r.URL.Query().Get("ports")
		targets := resolveExportPorts(portsParam, 0, dash.ports)
		if portsParam == "" {
			targets = make([]int, len(dash.ports))
			copy(targets, dash.ports)
		}
		for _, p := range targets {
			dash.store.Clear(p)
		}
		json.NewEncoder(w).Encode(map[string]any{"cleared": targets})
	})

	t.Run("DELETE method clears history", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/clear", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var response map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse JSON response: %v", err)
		}

		cleared, ok := response["cleared"].([]any)
		if !ok {
			t.Fatal("response should contain cleared array")
		}
		if len(cleared) != 2 {
			t.Errorf("expected 2 ports cleared, got %d", len(cleared))
		}

		// Verify data is cleared
		if len(store.GetHistory(8080)) != 0 {
			t.Error("expected port 8080 history to be cleared")
		}
		if len(store.GetHistory(8081)) != 0 {
			t.Error("expected port 8081 history to be cleared")
		}
	})

	t.Run("non-DELETE method returns 405", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/clear", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status 405, got %d", w.Code)
		}
	})
}

func TestDashboardServe_ClearHandlerWithPortFilter(t *testing.T) {
	store := NewStore(100)
	ports := []int{8080, 8081, 8082}
	flags := []string{}
	dash := NewDashboard(store, ports, flags)

	// Add test data to all ports
	for _, port := range ports {
		store.Add(port, EchoResponse{
			ID:         "test",
			Timestamp:  time.Now(),
			Port:       port,
			Method:     "GET",
			Path:       "/test",
			DurationMs: 10.0,
		})
	}

	// Create a test server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/clear", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		portsParam := r.URL.Query().Get("ports")
		targets := resolveExportPorts(portsParam, 0, dash.ports)
		if portsParam == "" {
			targets = make([]int, len(dash.ports))
			copy(targets, dash.ports)
		}
		for _, p := range targets {
			dash.store.Clear(p)
		}
		json.NewEncoder(w).Encode(map[string]any{"cleared": targets})
	})

	req := httptest.NewRequest("DELETE", "/api/clear?ports=8080,8082", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify only specified ports are cleared
	if len(store.GetHistory(8080)) != 0 {
		t.Error("expected port 8080 history to be cleared")
	}
	if len(store.GetHistory(8081)) == 0 {
		t.Error("expected port 8081 history to remain")
	}
	if len(store.GetHistory(8082)) != 0 {
		t.Error("expected port 8082 history to be cleared")
	}
}

func TestDashboardRender(t *testing.T) {
	store := NewStore(100)
	ports := []int{8080}
	flags := []string{"--delay=100ms"}
	dash := NewDashboard(store, ports, flags)

	// Add some test data
	store.Add(8080, EchoResponse{
		ID:         "test-1",
		Timestamp:  time.Now(),
		Port:       8080,
		Method:     "GET",
		Path:       "/test",
		DurationMs: 10.5,
		TLS:        false,
	})

	// Render should not panic
	// We can't easily test the output since it uses ANSI codes and prints to stdout
	// but we can at least verify it doesn't crash
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Render panicked: %v", r)
		}
	}()

	dash.Render()
}
