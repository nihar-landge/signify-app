package service

import (
	"log"
	"signify/internal/repository"
	"sync"
	"time"
)

// Message represents a single message in a conversation
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // "user" or "other"
	Content   string    `json:"content"`
	Type      string    `json:"type"` // "text", "audio", "sign"
	Timestamp time.Time `json:"timestamp"`
}

// Session represents an active user session
type Session struct {
	UserID         string
	ConversationID string
	Messages       []Message
	StartTime      time.Time
	LastActivity   time.Time
}

// SessionManager manages user sessions and conversation history
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	postgres *repository.PostgresRepository // optional, may be nil
}

// NewSessionManager creates a new session manager with optional PostgreSQL persistence
func NewSessionManager(pg *repository.PostgresRepository) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		postgres: pg,
	}
}

// CreateSession creates a new session for a user
func (sm *SessionManager) CreateSession(userID string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session := &Session{
		UserID:         userID,
		ConversationID: generateID(),
		Messages:       make([]Message, 0),
		StartTime:      time.Now(),
		LastActivity:   time.Now(),
	}

	sm.sessions[userID] = session
	log.Printf("Created session for user: %s", userID)

	// Persist conversation to PostgreSQL
	if sm.postgres != nil {
		go func() {
			if err := sm.postgres.SaveConversation(userID, session.ConversationID, "Conversation "+session.ConversationID); err != nil {
				log.Printf("Warning: failed to persist conversation to PostgreSQL: %v", err)
			}
		}()
	}

	return session
}

// GetSession retrieves a session by user ID
func (sm *SessionManager) GetSession(userID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[userID]
	return session, exists
}

// SaveMessage saves a message to the session
func (sm *SessionManager) SaveMessage(userID, role, content string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[userID]
	if !exists {
		// Create session if it doesn't exist
		session = &Session{
			UserID:         userID,
			ConversationID: generateID(),
			Messages:       make([]Message, 0),
			StartTime:      time.Now(),
		}
		sm.sessions[userID] = session
	}

	message := Message{
		ID:        generateID(),
		Role:      role,
		Content:   content,
		Type:      "text",
		Timestamp: time.Now(),
	}

	session.Messages = append(session.Messages, message)
	session.LastActivity = time.Now()

	log.Printf("Saved message for user %s: [%s] %s", userID, role, truncate(content, 50))

	// Persist to PostgreSQL asynchronously
	if sm.postgres != nil {
		convID := session.ConversationID
		go func() {
			if err := sm.postgres.SaveMessage(convID, message.ID, role, content, "text", 0); err != nil {
				log.Printf("Warning: failed to persist message to PostgreSQL: %v", err)
			}
		}()
	}
}

// GetMessages retrieves all messages for a user's current session
func (sm *SessionManager) GetMessages(userID string) []Message {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[userID]
	if !exists {
		return []Message{}
	}

	return session.Messages
}

// DeleteSession removes a session
func (sm *SessionManager) DeleteSession(userID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.sessions, userID)
	log.Printf("Deleted session for user: %s", userID)
}

// CleanupStaleSessions removes sessions inactive for more than the given duration
func (sm *SessionManager) CleanupStaleSessions(maxIdleTime time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for userID, session := range sm.sessions {
		if now.Sub(session.LastActivity) > maxIdleTime {
			delete(sm.sessions, userID)
			log.Printf("Cleaned up stale session for user: %s", userID)
		}
	}
}

// generateID generates a simple unique ID (in production, use UUID)
func generateID() string {
	return time.Now().Format("20060102150405.000000000")
}

// truncate truncates a string to the given length
func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}
