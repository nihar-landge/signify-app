package model

import (
	"time"
)

// Conversation represents a conversation between users
type Conversation struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Title     string    `json:"title" db:"title"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	Messages  []Message `json:"messages,omitempty"`
}

// Message represents a single message in a conversation
type Message struct {
	ID             string    `json:"id" db:"id"`
	ConversationID string    `json:"conversation_id" db:"conversation_id"`
	Role           string    `json:"role" db:"role"` // "user", "other", "system"
	Content        string    `json:"content" db:"content"`
	ContentType    string    `json:"content_type" db:"content_type"` // "text", "audio", "sign"
	Confidence     float32   `json:"confidence,omitempty" db:"confidence"`
	Timestamp      time.Time `json:"timestamp" db:"timestamp"`
}

// MessageType constants
const (
	MessageTypeText  = "text"
	MessageTypeAudio = "audio"
	MessageTypeSign  = "sign"
)

// MessageRole constants
const (
	RoleUser   = "user"
	RoleOther  = "other"
	RoleSystem = "system"
)
