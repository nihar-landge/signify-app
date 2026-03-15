package service

import (
	"testing"
	"time"
)

func TestNewSignStabilizer(t *testing.T) {
	s := NewSignStabilizer(5, 3, 1*time.Second)
	if s == nil {
		t.Fatal("expected non-nil stabilizer")
	}
}

func TestAddPrediction_ConsensusReached(t *testing.T) {
	s := NewSignStabilizer(5, 3, 0) // No cooldown for testing

	// Add 3 of the same prediction → should emit
	s.AddPrediction("session1", "hello", 0.9)
	s.AddPrediction("session1", "hello", 0.9)
	result := s.AddPrediction("session1", "hello", 0.9)

	if result != "hello" {
		t.Errorf("expected 'hello', got '%s'", result)
	}
}

func TestAddPrediction_NoConsensus(t *testing.T) {
	s := NewSignStabilizer(5, 3, 0)

	// Add mixed predictions → should not emit
	s.AddPrediction("session1", "hello", 0.9)
	s.AddPrediction("session1", "world", 0.9)
	result := s.AddPrediction("session1", "bye", 0.9)

	if result != "" {
		t.Errorf("expected empty string (no consensus), got '%s'", result)
	}
}

func TestAddPrediction_Cooldown(t *testing.T) {
	s := NewSignStabilizer(5, 3, 2*time.Second)

	// First consensus
	s.AddPrediction("session1", "hello", 0.9)
	s.AddPrediction("session1", "hello", 0.9)
	result1 := s.AddPrediction("session1", "hello", 0.9)
	if result1 != "hello" {
		t.Errorf("expected 'hello' on first consensus, got '%s'", result1)
	}

	// Try again immediately → should be in cooldown
	s.AddPrediction("session1", "hello", 0.9)
	s.AddPrediction("session1", "hello", 0.9)
	result2 := s.AddPrediction("session1", "hello", 0.9)
	if result2 != "" {
		t.Errorf("expected empty string during cooldown, got '%s'", result2)
	}
}

func TestAddPrediction_DifferentSessions(t *testing.T) {
	s := NewSignStabilizer(5, 3, 0)

	// Session 1
	s.AddPrediction("session1", "hello", 0.9)
	s.AddPrediction("session1", "hello", 0.9)
	result1 := s.AddPrediction("session1", "hello", 0.9)

	// Session 2
	s.AddPrediction("session2", "world", 0.9)
	s.AddPrediction("session2", "world", 0.9)
	result2 := s.AddPrediction("session2", "world", 0.9)

	if result1 != "hello" {
		t.Errorf("session1: expected 'hello', got '%s'", result1)
	}
	if result2 != "world" {
		t.Errorf("session2: expected 'world', got '%s'", result2)
	}
}

func TestClearSession(t *testing.T) {
	s := NewSignStabilizer(5, 3, 0)

	// Build up state
	s.AddPrediction("session1", "hello", 0.9)
	s.AddPrediction("session1", "hello", 0.9)

	// Clear
	s.ClearSession("session1")

	// Should need full 3 predictions again
	s.AddPrediction("session1", "hello", 0.9)
	result := s.AddPrediction("session1", "hello", 0.9)
	if result != "" {
		t.Errorf("expected empty after clear, got '%s'", result)
	}
}

func TestWindowSliding(t *testing.T) {
	s := NewSignStabilizer(3, 2, 0) // Window of 3, need 2 to match

	// Add 2 "hello" → consensus reached (2 >= min of 2)
	s.AddPrediction("s1", "hello", 0.9)
	result := s.AddPrediction("s1", "hello", 0.9) // emits and clears
	if result != "hello" {
		t.Errorf("expected 'hello', got '%s'", result)
	}

	// Now add different predictions — window was cleared after emit
	s.AddPrediction("s1", "world", 0.9)
	result = s.AddPrediction("s1", "bye", 0.9)
	if result != "" {
		t.Errorf("expected empty (mixed window), got '%s'", result)
	}
}
