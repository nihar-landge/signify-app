package service

import (
	"sync"
	"time"
)

// SignStabilizer smoothes out raw sign predictions to prevent flickering
type SignStabilizer struct {
	windowSize     int
	minOccurrences int
	cooldown       time.Duration

	// State
	sessions map[string]*stabilizerState
	mu       sync.Mutex
}

type stabilizerState struct {
	recentPredictions []string
	lastEmitTime      time.Time
}

// NewSignStabilizer creates a new stabilizer
// windowSize: how many recent frames to consider (e.g., 5)
// minOccurrences: how many times a sign must appear in window to be valid (e.g., 3)
// cooldown: min time between emitting the same sign (e.g., 1s)
func NewSignStabilizer(windowSize, minOccurrences int, cooldown time.Duration) *SignStabilizer {
	return &SignStabilizer{
		windowSize:     windowSize,
		minOccurrences: minOccurrences,
		cooldown:       cooldown,
		sessions:       make(map[string]*stabilizerState),
	}
}

// AddPrediction processes a new prediction and returns a stable sign if ready
// Returns empty string if no stable sign is found or if in cooldown
func (s *SignStabilizer) AddPrediction(sessionID, label string, confidence float32) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.sessions[sessionID]
	if !exists {
		state = &stabilizerState{
			recentPredictions: make([]string, 0, s.windowSize),
		}
		s.sessions[sessionID] = state
	}

	// Add to window
	state.recentPredictions = append(state.recentPredictions, label)
	if len(state.recentPredictions) > s.windowSize {
		// Keep last N
		state.recentPredictions = state.recentPredictions[len(state.recentPredictions)-s.windowSize:]
	}

	// Count occurrences
	counts := make(map[string]int)
	for _, p := range state.recentPredictions {
		counts[p]++
	}

	// Find most frequent
	var bestLabel string
	var maxCount int
	for l, c := range counts {
		if c > maxCount {
			maxCount = c
			bestLabel = l
		}
	}

	// Check criteria
	if maxCount >= s.minOccurrences {
		// Consensus reached

		// Check cooldown (prevent repeating same word too fast)
		if time.Since(state.lastEmitTime) > s.cooldown {
			state.lastEmitTime = time.Now()
			// Clear window after valid emit to prevent double counting
			state.recentPredictions = state.recentPredictions[:0]
			return bestLabel
		}
	}

	return ""
}

// ClearSession removes session state
func (s *SignStabilizer) ClearSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}
