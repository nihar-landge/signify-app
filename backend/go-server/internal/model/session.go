package model

import (
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
)

// CallSessionStatus represents the current state of a call session
type CallSessionStatus string

const (
	CallSessionStatusCreated   CallSessionStatus = "created"
	CallSessionStatusDialing   CallSessionStatus = "dialing"
	CallSessionStatusRinging   CallSessionStatus = "ringing"
	CallSessionStatusAnswered  CallSessionStatus = "answered"
	CallSessionStatusStreaming CallSessionStatus = "streaming"
	CallSessionStatusCompleted CallSessionStatus = "completed"
	CallSessionStatusFailed    CallSessionStatus = "failed"
)

// LandmarkPacket represents the v1 landmark packet from the Over app
type LandmarkPacket struct {
	SessionID  string    `json:"sessionId"`
	Seq        int       `json:"seq"`
	TsMs       int64     `json:"ts_ms"`
	Present    bool      `json:"present"`
	Landmarks  []float64 `json:"landmarks"` // 63 floats (21 points × 3 coords) when present=true
	Normalized bool      `json:"normalized"`
}

// CallSession represents an active call session with Twilio
type CallSession struct {
	ID            string            `json:"id"`
	ToPhoneNumber string            `json:"to_phone_number"`
	CallSid       string            `json:"call_sid"`
	StreamSid     string            `json:"stream_sid"`
	Status        CallSessionStatus `json:"status"`
	Language      string            `json:"language"`       // "ASL" or "ISL"
	AudioEncoding string            `json:"audio_encoding"` // "mulaw" or "linear16"
	SampleRate    int               `json:"sample_rate"`    // 8000 or 16000

	// STTLanguage is the BCP-47 language code for Deepgram (e.g. "en-US", "hi")
	STTLanguage string `json:"stt_language"`

	// AudioIn receives decoded audio frames from telephony provider
	AudioIn chan []byte `json:"-"`

	// AudioOut sends encoded audio frames (Mulaw/PCM) to telephony provider
	AudioOut chan []byte `json:"-"`

	// AppEventsConn is the WebSocket connection to push events to the Over app
	AppEventsConn *websocket.Conn `json:"-"`

	// STTActive indicates if STT worker is running for this session
	STTActive bool `json:"-"`

	// STTStopChan signals STT worker to stop
	STTStopChan chan struct{} `json:"-"`

	// Landmark streaming (Milestone 3)
	LandmarksIn    chan LandmarkPacket `json:"-"` // Incoming landmarks from app
	LandmarksConn  *websocket.Conn     `json:"-"` // WebSocket for landmarks
	LandmarkWindow []LandmarkPacket    `json:"-"` // Sliding window (max 30 frames)
	LandmarkMu     sync.Mutex          `json:"-"` // Mutex for landmark window
	LandmarkSeq    int                 `json:"-"` // Last received sequence number
	LandmarkActive bool                `json:"-"` // If landmark worker is running
	LandmarkStop   chan struct{}       `json:"-"` // Stop signal for landmark worker

	// Mutex for thread-safe access
	mu sync.RWMutex `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewCallSession creates a new session with initialized channels
func NewCallSession(id, toPhoneNumber string) *CallSession {
	now := time.Now()
	return &CallSession{
		ID:             id,
		ToPhoneNumber:  toPhoneNumber,
		Status:         CallSessionStatusCreated,
		STTLanguage:    "en-US",                // Default; overridden per-connection by HandleAppAudioWS
		AudioIn:        make(chan []byte, 500), // Large buffer for audio frames
		AudioOut:       make(chan []byte, 500), // Large buffer for TTS/Playback frames
		STTStopChan:    make(chan struct{}),
		LandmarksIn:    make(chan LandmarkPacket, 50), // Buffer for landmark packets
		LandmarkWindow: make([]LandmarkPacket, 0, 30), // Sliding window (max 30 frames)
		LandmarkStop:   make(chan struct{}),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// Close safely closes the session's channels and connections
func (s *CallSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop STT worker
	if s.STTStopChan != nil {
		select {
		case <-s.STTStopChan:
			// Already closed
		default:
			close(s.STTStopChan)
		}
	}
	s.STTActive = false

	// Stop landmark worker
	if s.LandmarkActive {
		close(s.LandmarkStop)
		s.LandmarkActive = false
	}

	// Close audio channels
	if s.AudioIn != nil {
		close(s.AudioIn)
		s.AudioIn = nil
	}
	if s.AudioOut != nil {
		close(s.AudioOut)
		s.AudioOut = nil
	}

	// Close landmarks channel
	if s.LandmarksIn != nil {
		close(s.LandmarksIn)
		s.LandmarksIn = nil
	}

	// Close app events connection
	if s.AppEventsConn != nil {
		s.AppEventsConn.Close()
		s.AppEventsConn = nil
	}

	// Close landmarks connection
	if s.LandmarksConn != nil {
		s.LandmarksConn.Close()
		s.LandmarksConn = nil
	}
}

// ResetSTTState reinitializes the STT stop channel so a new STT worker
// can be started for the same session (e.g. when the Flutter mic WS
// reconnects after the user releases and re-presses Hold-to-Speak).
// If a worker is still running it is signalled to stop first.
func (s *CallSession) ResetSTTState() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Signal any currently-running worker to stop.
	if s.STTActive && s.STTStopChan != nil {
		select {
		case <-s.STTStopChan:
			// Already closed – nothing to do.
		default:
			close(s.STTStopChan)
		}
	}

	// Reinitialise so TryStartSTTWorker passes on the next call.
	s.STTStopChan = make(chan struct{})
	s.STTActive = false
}

// TryStartSTTWorker marks STT worker active if not already running.
// Returns true if caller should start a worker goroutine.
func (s *CallSession) TryStartSTTWorker() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.STTActive {
		return false
	}

	if s.STTStopChan != nil {
		select {
		case <-s.STTStopChan:
			// Stop requested; do not start new workers.
			return false
		default:
		}
	}

	s.STTActive = true
	return true
}

// MarkSTTWorkerStopped marks STT worker as not running.
func (s *CallSession) MarkSTTWorkerStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.STTActive = false
}

// RequestSTTStop signals the STT worker to stop.
func (s *CallSession) RequestSTTStop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.STTStopChan == nil {
		return
	}

	select {
	case <-s.STTStopChan:
		// Already closed
	default:
		close(s.STTStopChan)
	}
}

// IsSTTStopRequested returns true if stop has been signaled.
func (s *CallSession) IsSTTStopRequested() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.STTStopChan == nil {
		return true
	}

	select {
	case <-s.STTStopChan:
		return true
	default:
		return false
	}
}

// PushAudio sends audio data to the session's AudioIn channel
// Uses non-blocking send to prevent WebSocket deadlock
// Returns true if audio was queued, false if dropped (buffer full)
func (s *CallSession) PushAudio(audio []byte) bool {
	select {
	case s.AudioIn <- audio:
		return true
	default:
		// Buffer full, drop frame to prevent blocking
		return false
	}
}

// SetAppEventsConn sets the app events WebSocket connection
func (s *CallSession) SetAppEventsConn(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close old connection if exists
	if s.AppEventsConn != nil {
		s.AppEventsConn.Close()
	}
	s.AppEventsConn = conn
}

// SendAppEvent sends a JSON event to the app via WebSocket
// Returns false if no connection or send failed
func (s *CallSession) SendAppEvent(event interface{}) bool {
	s.mu.RLock()
	eventsConn := s.AppEventsConn
	landmarksConn := s.LandmarksConn
	s.mu.RUnlock()

	// Try events connection first
	if eventsConn != nil {
		err := eventsConn.WriteJSON(event)
		if err == nil {
			return true
		}
	}

	// Fallback to landmarks connection (useful for Test Mode)
	if landmarksConn != nil {
		err := landmarksConn.WriteJSON(event)
		return err == nil
	}

	return false
}

// SetLandmarksConn sets the landmarks WebSocket connection
func (s *CallSession) SetLandmarksConn(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close old connection if exists
	if s.LandmarksConn != nil {
		s.LandmarksConn.Close()
	}
	s.LandmarksConn = conn
}

// PushLandmark adds a landmark packet to the sliding window
// Maintains max 30 frames (FIFO)
func (s *CallSession) PushLandmark(pkt LandmarkPacket) {
	s.LandmarkMu.Lock()
	defer s.LandmarkMu.Unlock()

	s.LandmarkWindow = append(s.LandmarkWindow, pkt)

	// Keep only last 30 frames
	if len(s.LandmarkWindow) > 30 {
		s.LandmarkWindow = s.LandmarkWindow[len(s.LandmarkWindow)-30:]
	}

	s.LandmarkSeq = pkt.Seq
}

// GetLandmarkWindow returns a copy of the current landmark window
func (s *CallSession) GetLandmarkWindow() []LandmarkPacket {
	s.LandmarkMu.Lock()
	defer s.LandmarkMu.Unlock()

	result := make([]LandmarkPacket, len(s.LandmarkWindow))
	copy(result, s.LandmarkWindow)
	return result
}

// ClearLandmarkWindow clears the landmark window
func (s *CallSession) ClearLandmarkWindow() {
	s.LandmarkMu.Lock()
	defer s.LandmarkMu.Unlock()
	s.LandmarkWindow = s.LandmarkWindow[:0]
}
