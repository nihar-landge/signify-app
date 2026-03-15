package model

import (
	"time"
)

// User represents an application user
type User struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Email     string    `json:"email,omitempty" db:"email"`
	Settings  Settings  `json:"settings,omitempty"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Settings represents user preferences
type Settings struct {
	Language      string  `json:"language"`        // Preferred language (e.g., "en", "es")
	VoiceSpeed    float32 `json:"voice_speed"`     // TTS speed multiplier
	VoicePitch    float32 `json:"voice_pitch"`     // TTS pitch
	CaptionSize   int     `json:"caption_size"`    // Font size for captions
	AutoPlayAudio bool    `json:"auto_play_audio"` // Auto-play TTS output
}

// DefaultSettings returns default user settings
func DefaultSettings() Settings {
	return Settings{
		Language:      "en",
		VoiceSpeed:    1.0,
		VoicePitch:    1.0,
		CaptionSize:   16,
		AutoPlayAudio: true,
	}
}

// Session represents an active WebSocket session
type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	ConnectedAt  time.Time `json:"connected_at"`
	LastActivity time.Time `json:"last_activity"`
	Mode         string    `json:"mode"` // "conversation", "call"
}

// SessionMode constants
const (
	ModeConversation = "conversation"
	ModeCall         = "call"
)
