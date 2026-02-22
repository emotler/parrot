package main

import (
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	maxSize := 50
	store := NewStore(maxSize)

	if store == nil {
		t.Fatal("NewStore returned nil")
	}
	if store.maxSize != maxSize {
		t.Errorf("expected maxSize %d, got %d", maxSize, store.maxSize)
	}
	if store.history == nil {
		t.Error("history map not initialized")
	}
	if store.counts == nil {
		t.Error("counts map not initialized")
	}
	if store.totalMs == nil {
		t.Error("totalMs map not initialized")
	}
}

func TestStoreAdd(t *testing.T) {
	store := NewStore(10)
	port := 8080

	resp := EchoResponse{
		ID:         "test-id-1",
		Timestamp:  time.Now(),
		Port:       port,
		Method:     "GET",
		Path:       "/test",
		DurationMs: 15.5,
	}

	store.Add(port, resp)

	history := store.GetHistory(port)
	if len(history) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(history))
	}
	if history[0].ID != "test-id-1" {
		t.Errorf("expected ID test-id-1, got %s", history[0].ID)
	}

	count, avgMs := store.Stats(port)
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
	if avgMs != 15.5 {
		t.Errorf("expected avgMs 15.5, got %f", avgMs)
	}
}

func TestStoreAddMultiple(t *testing.T) {
	store := NewStore(10)
	port := 8080

	for i := 0; i < 5; i++ {
		resp := EchoResponse{
			ID:         "test-id",
			Timestamp:  time.Now(),
			Port:       port,
			DurationMs: 10.0,
		}
		store.Add(port, resp)
	}

	history := store.GetHistory(port)
	if len(history) != 5 {
		t.Errorf("expected 5 entries, got %d", len(history))
	}

	count, avgMs := store.Stats(port)
	if count != 5 {
		t.Errorf("expected count 5, got %d", count)
	}
	if avgMs != 10.0 {
		t.Errorf("expected avgMs 10.0, got %f", avgMs)
	}
}

func TestStoreMaxSize(t *testing.T) {
	maxSize := 3
	store := NewStore(maxSize)
	port := 8080

	// Add more entries than maxSize
	for i := 0; i < 5; i++ {
		resp := EchoResponse{
			ID:         "id-" + string(rune('0'+i)),
			Timestamp:  time.Now(),
			Port:       port,
			DurationMs: float64(i),
		}
		store.Add(port, resp)
	}

	history := store.GetHistory(port)
	if len(history) != maxSize {
		t.Errorf("expected history size %d, got %d", maxSize, len(history))
	}

	// Should keep the last 3 entries (id-2, id-3, id-4)
	if history[0].ID != "id-2" {
		t.Errorf("expected first entry id-2, got %s", history[0].ID)
	}
	if history[2].ID != "id-4" {
		t.Errorf("expected last entry id-4, got %s", history[2].ID)
	}

	// Count should still be 5 (total requests)
	count, _ := store.Stats(port)
	if count != 5 {
		t.Errorf("expected count 5, got %d", count)
	}
}

func TestStoreMultiplePorts(t *testing.T) {
	store := NewStore(10)
	port1 := 8080
	port2 := 8081

	resp1 := EchoResponse{ID: "port1-req", Port: port1, DurationMs: 10.0}
	resp2 := EchoResponse{ID: "port2-req", Port: port2, DurationMs: 20.0}

	store.Add(port1, resp1)
	store.Add(port2, resp2)

	history1 := store.GetHistory(port1)
	history2 := store.GetHistory(port2)

	if len(history1) != 1 || history1[0].ID != "port1-req" {
		t.Error("port1 history incorrect")
	}
	if len(history2) != 1 || history2[0].ID != "port2-req" {
		t.Error("port2 history incorrect")
	}

	count1, avg1 := store.Stats(port1)
	count2, avg2 := store.Stats(port2)

	if count1 != 1 || avg1 != 10.0 {
		t.Errorf("port1 stats incorrect: count=%d, avg=%f", count1, avg1)
	}
	if count2 != 1 || avg2 != 20.0 {
		t.Errorf("port2 stats incorrect: count=%d, avg=%f", count2, avg2)
	}
}

func TestStoreGetHistory(t *testing.T) {
	store := NewStore(10)
	port := 8080

	// Empty history
	history := store.GetHistory(port)
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}

	// Add entry
	resp := EchoResponse{ID: "test", Port: port}
	store.Add(port, resp)

	history = store.GetHistory(port)
	if len(history) != 1 {
		t.Errorf("expected 1 entry, got %d", len(history))
	}

	// Verify it returns a copy (not the internal slice)
	history[0].ID = "modified"
	history2 := store.GetHistory(port)
	if history2[0].ID == "modified" {
		t.Error("GetHistory should return a copy, not the internal slice")
	}
}

func TestStoreStats(t *testing.T) {
	store := NewStore(10)
	port := 8080

	// Empty stats
	count, avgMs := store.Stats(port)
	if count != 0 || avgMs != 0 {
		t.Errorf("expected zero stats, got count=%d, avg=%f", count, avgMs)
	}

	// Add entries with different durations
	store.Add(port, EchoResponse{DurationMs: 10.0})
	store.Add(port, EchoResponse{DurationMs: 20.0})
	store.Add(port, EchoResponse{DurationMs: 30.0})

	count, avgMs = store.Stats(port)
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
	expectedAvg := 20.0
	if avgMs != expectedAvg {
		t.Errorf("expected avg %f, got %f", expectedAvg, avgMs)
	}
}

func TestStoreUptime(t *testing.T) {
	store := NewStore(10)
	time.Sleep(10 * time.Millisecond)
	uptime := store.Uptime()

	if uptime < 10*time.Millisecond {
		t.Errorf("expected uptime >= 10ms, got %v", uptime)
	}
	if uptime > 1*time.Second {
		t.Errorf("expected uptime < 1s, got %v", uptime)
	}
}

func TestStoreClear(t *testing.T) {
	store := NewStore(10)
	port := 8080

	// Add some entries
	for i := 0; i < 3; i++ {
		store.Add(port, EchoResponse{ID: "test", DurationMs: 10.0})
	}

	count, avgMs := store.Stats(port)
	if count != 3 {
		t.Fatalf("expected count 3 before clear, got %d", count)
	}

	// Clear
	store.Clear(port)

	// Verify cleared
	history := store.GetHistory(port)
	if len(history) != 0 {
		t.Errorf("expected empty history after clear, got %d entries", len(history))
	}

	count, avgMs = store.Stats(port)
	if count != 0 || avgMs != 0 {
		t.Errorf("expected zero stats after clear, got count=%d, avg=%f", count, avgMs)
	}
}

func TestStoreClearOnlyAffectsTargetPort(t *testing.T) {
	store := NewStore(10)
	port1 := 8080
	port2 := 8081

	store.Add(port1, EchoResponse{ID: "port1"})
	store.Add(port2, EchoResponse{ID: "port2"})

	store.Clear(port1)

	history1 := store.GetHistory(port1)
	history2 := store.GetHistory(port2)

	if len(history1) != 0 {
		t.Error("port1 should be cleared")
	}
	if len(history2) != 1 {
		t.Error("port2 should not be affected")
	}
}

func TestStoreGetByID(t *testing.T) {
	store := NewStore(10)
	port := 8080

	resp := EchoResponse{
		ID:     "unique-id-123",
		Port:   port,
		Method: "POST",
		Path:   "/webhook",
	}
	store.Add(port, resp)

	// Find existing
	found, ok := store.GetByID("unique-id-123")
	if !ok {
		t.Fatal("expected to find entry by ID")
	}
	if found.Method != "POST" || found.Path != "/webhook" {
		t.Errorf("found entry has wrong data: %+v", found)
	}

	// Not found
	_, ok = store.GetByID("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent ID")
	}
}

func TestStoreGetByIDMultiplePorts(t *testing.T) {
	store := NewStore(10)

	store.Add(8080, EchoResponse{ID: "id-8080", Port: 8080})
	store.Add(8081, EchoResponse{ID: "id-8081", Port: 8081})
	store.Add(8082, EchoResponse{ID: "id-8082", Port: 8082})

	// Should find across all ports
	found, ok := store.GetByID("id-8081")
	if !ok || found.Port != 8081 {
		t.Error("should find entry from port 8081")
	}
}

func TestStoreAllPorts(t *testing.T) {
	store := NewStore(10)

	// Empty
	ports := store.AllPorts()
	if len(ports) != 0 {
		t.Errorf("expected no ports, got %v", ports)
	}

	// Add entries to multiple ports
	store.Add(8080, EchoResponse{})
	store.Add(8081, EchoResponse{})
	store.Add(8080, EchoResponse{}) // duplicate port

	ports = store.AllPorts()
	if len(ports) != 2 {
		t.Errorf("expected 2 unique ports, got %d: %v", len(ports), ports)
	}

	// Verify both ports are present (order doesn't matter)
	portMap := make(map[int]bool)
	for _, p := range ports {
		portMap[p] = true
	}
	if !portMap[8080] || !portMap[8081] {
		t.Errorf("expected ports 8080 and 8081, got %v", ports)
	}
}

func TestStoreConcurrency(t *testing.T) {
	store := NewStore(100)
	port := 8080
	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				store.Add(port, EchoResponse{
					ID:         "concurrent",
					DurationMs: 5.0,
				})
			}
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				store.GetHistory(port)
				store.Stats(port)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}

	count, _ := store.Stats(port)
	if count != 100 {
		t.Errorf("expected count 100, got %d", count)
	}
}
