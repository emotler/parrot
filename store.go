package main

import (
	"sync"
	"time"
)

// Store holds per-port request history and aggregate stats.
type Store struct {
	mu        sync.RWMutex
	history   map[int][]EchoResponse
	counts    map[int]int64
	totalMs   map[int]float64
	maxSize   int
	startTime time.Time
}

func NewStore(maxSize int) *Store {
	return &Store{
		history:   make(map[int][]EchoResponse),
		counts:    make(map[int]int64),
		totalMs:   make(map[int]float64),
		maxSize:   maxSize,
		startTime: time.Now(),
	}
}

func (s *Store) Add(port int, r EchoResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history[port] = append(s.history[port], r)
	if len(s.history[port]) > s.maxSize {
		s.history[port] = s.history[port][len(s.history[port])-s.maxSize:]
	}
	s.counts[port]++
	s.totalMs[port] += r.DurationMs
}

func (s *Store) GetHistory(port int) []EchoResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := s.history[port]
	out := make([]EchoResponse, len(h))
	copy(out, h)
	return out
}

func (s *Store) Stats(port int) (count int64, avgMs float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count = s.counts[port]
	if count > 0 {
		avgMs = s.totalMs[port] / float64(count)
	}
	return
}

func (s *Store) Uptime() time.Duration {
	return time.Since(s.startTime)
}

func (s *Store) Clear(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history[port] = nil
	s.counts[port] = 0
	s.totalMs[port] = 0
}

func (s *Store) GetByID(id string) (EchoResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entries := range s.history {
		for _, e := range entries {
			if e.ID == id {
				return e, true
			}
		}
	}
	return EchoResponse{}, false
}

func (s *Store) AllPorts() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ports := make([]int, 0, len(s.counts))
	for p := range s.counts {
		ports = append(ports, p)
	}
	return ports
}
