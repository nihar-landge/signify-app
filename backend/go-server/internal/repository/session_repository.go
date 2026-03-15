package repository

import (
	"fmt"
	"signify/internal/model"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CallSessionRepository manages call sessions in memory
type CallSessionRepository struct {
	sessions map[string]*model.CallSession
	mu       sync.RWMutex
}

// NewCallSessionRepository creates a new session repository
func NewCallSessionRepository() *CallSessionRepository {
	return &CallSessionRepository{
		sessions: make(map[string]*model.CallSession),
	}
}

// CreateSession creates a new session and returns it
func (r *CallSessionRepository) CreateSession(toPhoneNumber string) (*model.CallSession, error) {
	sessionID := uuid.New().String()
	session := model.NewCallSession(sessionID, toPhoneNumber)

	r.mu.Lock()
	r.sessions[sessionID] = session
	r.mu.Unlock()

	return session, nil
}

// AddSession adds a pre-created session to the repository
func (r *CallSessionRepository) AddSession(session *model.CallSession) {
	r.mu.Lock()
	r.sessions[session.ID] = session
	r.mu.Unlock()
}

// GetSession retrieves a session by ID
func (r *CallSessionRepository) GetSession(id string) (*model.CallSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, exists := r.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return session, nil
}

// UpdateSession updates an existing session
func (r *CallSessionRepository) UpdateSession(session *model.CallSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[session.ID]; !exists {
		return fmt.Errorf("session not found: %s", session.ID)
	}

	session.UpdatedAt = time.Now()
	r.sessions[session.ID] = session
	return nil
}

// SetCallSid sets the Twilio call SID for a session
func (r *CallSessionRepository) SetCallSid(sessionID, callSid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.CallSid = callSid
	session.UpdatedAt = time.Now()
	return nil
}

// SetStreamSid sets the Twilio stream SID for a session
func (r *CallSessionRepository) SetStreamSid(sessionID, streamSid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.StreamSid = streamSid
	session.UpdatedAt = time.Now()
	return nil
}

// SetStatus updates the session status
func (r *CallSessionRepository) SetStatus(sessionID string, status model.CallSessionStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Status = status
	session.UpdatedAt = time.Now()
	return nil
}

// DeleteSession removes a session
func (r *CallSessionRepository) DeleteSession(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if session, exists := r.sessions[id]; exists {
		session.Close()
		delete(r.sessions, id)
	}
}

// GetSessionByCallSid finds a session by its Twilio call SID
func (r *CallSessionRepository) GetSessionByCallSid(callSid string) (*model.CallSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, session := range r.sessions {
		if session.CallSid == callSid {
			return session, nil
		}
	}
	return nil, fmt.Errorf("session not found for call SID: %s", callSid)
}

// GetAllSessions returns all active sessions
func (r *CallSessionRepository) GetAllSessions() []*model.CallSession {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sessions := make([]*model.CallSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// CleanupOldSessions removes sessions older than the given duration
func (r *CallSessionRepository) CleanupOldSessions(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, session := range r.sessions {
		if session.UpdatedAt.Before(cutoff) {
			session.Close()
			delete(r.sessions, id)
			removed++
		}
	}

	return removed
}
