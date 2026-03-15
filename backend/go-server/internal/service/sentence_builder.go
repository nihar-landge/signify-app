package service

import (
	"strings"
	"sync"
	"time"
)

// SentenceBuilder accumulates words into a sentence based on a timeout
type SentenceBuilder struct {
	mu        sync.Mutex
	buffer    []string
	timer     *time.Timer
	timeout   time.Duration
	callbacks map[string]SentenceCallback // Map of sessionID -> callback
}

// SentenceCallback is called when a sentence updates
// text: the full sentence so far
// isFinal: true if the sentence is complete (timeout reached)
type SentenceCallback func(text string, isFinal bool)

// NewSentenceBuilder creates a new builder
// timeout: how long to wait after the last word before finalizing the sentence
func NewSentenceBuilder(timeout time.Duration) *SentenceBuilder {
	return &SentenceBuilder{
		timeout:   timeout,
		callbacks: make(map[string]SentenceCallback),
		buffer:    make([]string, 0),
	}
}

// RegisterCallback registers a function to be called when session sentence updates
// Note: Currently simple implementation, assuming one global builder or session-managed.
// For multi-session, it's better to having one builder per session or map logic.
// Refactoring to be per-session instance in CallHandler is cleaner, but to match Stabilizer pattern:
// We will maintain state per session here.

type sessionSentenceState struct {
	buffer            []string
	timer             *time.Timer
	lastFinalSentence string
}

type MultiSessionSentenceBuilder struct {
	mu        sync.Mutex
	timeout   time.Duration
	sessions  map[string]*sessionSentenceState
	callbacks map[string]SentenceCallback
}

func NewMultiSessionSentenceBuilder(timeout time.Duration) *MultiSessionSentenceBuilder {
	return &MultiSessionSentenceBuilder{
		timeout:   timeout,
		sessions:  make(map[string]*sessionSentenceState),
		callbacks: make(map[string]SentenceCallback),
	}
}

// RegisterCallback sets the callback for a specific session
func (s *MultiSessionSentenceBuilder) RegisterCallback(sessionID string, cb SentenceCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks[sessionID] = cb
}

// AddWord adds a stabilized word to the session's sentence
func (s *MultiSessionSentenceBuilder) AddWord(sessionID string, word string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.sessions[sessionID]
	if !exists {
		state = &sessionSentenceState{
			buffer: make([]string, 0),
		}
		s.sessions[sessionID] = state
	}

	// Prevent consecutive duplicates
	if len(state.buffer) > 0 && state.buffer[len(state.buffer)-1] == word {
		// Stop existing timer if running (debounce) - we still want to reset the timeout
		if state.timer != nil {
			state.timer.Stop()
		}
		// Start timer for FINAL
		state.timer = time.AfterFunc(s.timeout, func() {
			s.finalize(sessionID)
		})
		return
	}

	// Stop existing timer if running (debounce)
	if state.timer != nil {
		state.timer.Stop()
	}

	// Add word
	state.buffer = append(state.buffer, word)
	currentSentence := strings.Join(state.buffer, " ")
	state.lastFinalSentence = "" // Reset last final as we are building a NEW sentence

	// Emit PARTIAL
	if cb, ok := s.callbacks[sessionID]; ok {
		go cb(currentSentence, false)
	}

	// Start timer for FINAL
	state.timer = time.AfterFunc(s.timeout, func() {
		s.finalize(sessionID)
	})
}

// finalize is called by the timer
func (s *MultiSessionSentenceBuilder) finalize(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.sessions[sessionID]
	if !exists || len(state.buffer) == 0 {
		return
	}

	finalSentence := strings.Join(state.buffer, " ")

	// Deduplication: Don't repeat the exact same final sentence twice in a row
	if finalSentence == state.lastFinalSentence {
		state.buffer = state.buffer[:0]
		state.timer = nil
		return
	}

	state.lastFinalSentence = finalSentence

	// Clear buffer (start fresh for next sentence)
	state.buffer = state.buffer[:0]
	state.timer = nil

	// Emit FINAL
	if cb, ok := s.callbacks[sessionID]; ok {
		go cb(finalSentence, true)
	}
}

// ClearSession removes session state
func (s *MultiSessionSentenceBuilder) ClearSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state, ok := s.sessions[sessionID]; ok {
		if state.timer != nil {
			state.timer.Stop()
		}
		delete(s.sessions, sessionID)
	}
	delete(s.callbacks, sessionID)
}
