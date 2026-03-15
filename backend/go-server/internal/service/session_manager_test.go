package service

import (
	"testing"
	"time"
)

func TestNewSessionManager_NoPostgres(t *testing.T) {
	sm := NewSessionManager(nil)
	if sm == nil {
		t.Fatal("expected non-nil session manager")
	}
}

func TestCreateSession(t *testing.T) {
	sm := NewSessionManager(nil)

	session := sm.CreateSession("user1")
	if session.UserID != "user1" {
		t.Errorf("expected UserID 'user1', got '%s'", session.UserID)
	}
	if session.ConversationID == "" {
		t.Error("expected non-empty ConversationID")
	}
	if len(session.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(session.Messages))
	}
}

func TestGetSession(t *testing.T) {
	sm := NewSessionManager(nil)

	// Before creating
	_, exists := sm.GetSession("user1")
	if exists {
		t.Error("expected session to not exist before creation")
	}

	// After creating
	sm.CreateSession("user1")
	session, exists := sm.GetSession("user1")
	if !exists {
		t.Error("expected session to exist after creation")
	}
	if session.UserID != "user1" {
		t.Errorf("expected UserID 'user1', got '%s'", session.UserID)
	}
}

func TestSaveMessage(t *testing.T) {
	sm := NewSessionManager(nil)

	sm.SaveMessage("user1", "user", "Hello world")
	messages := sm.GetMessages("user1")
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Role != "user" {
		t.Errorf("expected role 'user', got '%s'", messages[0].Role)
	}
	if messages[0].Content != "Hello world" {
		t.Errorf("expected content 'Hello world', got '%s'", messages[0].Content)
	}
}

func TestSaveMessage_AutoCreateSession(t *testing.T) {
	sm := NewSessionManager(nil)

	// Save message without creating session first
	sm.SaveMessage("user2", "other", "Goodbye")

	session, exists := sm.GetSession("user2")
	if !exists {
		t.Error("expected session to be auto-created")
	}
	if len(session.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(session.Messages))
	}
}

func TestDeleteSession(t *testing.T) {
	sm := NewSessionManager(nil)

	sm.CreateSession("user1")
	sm.DeleteSession("user1")

	_, exists := sm.GetSession("user1")
	if exists {
		t.Error("expected session to not exist after deletion")
	}
}

func TestGetMessages_NoSession(t *testing.T) {
	sm := NewSessionManager(nil)

	messages := sm.GetMessages("nonexistent")
	if len(messages) != 0 {
		t.Errorf("expected 0 messages for nonexistent user, got %d", len(messages))
	}
}

func TestCleanupStaleSessions(t *testing.T) {
	sm := NewSessionManager(nil)

	session := sm.CreateSession("user1")
	// Simulate old activity
	session.LastActivity = time.Now().Add(-2 * time.Hour)

	sm.CleanupStaleSessions(1 * time.Hour)

	_, exists := sm.GetSession("user1")
	if exists {
		t.Error("expected stale session to be cleaned up")
	}
}

func TestMultipleMessages(t *testing.T) {
	sm := NewSessionManager(nil)

	sm.CreateSession("user1")
	sm.SaveMessage("user1", "user", "Hello")
	sm.SaveMessage("user1", "other", "Hi there")
	sm.SaveMessage("user1", "user", "How are you?")

	messages := sm.GetMessages("user1")
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].Content != "Hello" {
		t.Errorf("expected first message 'Hello', got '%s'", messages[0].Content)
	}
	if messages[2].Content != "How are you?" {
		t.Errorf("expected third message 'How are you?', got '%s'", messages[2].Content)
	}
}
