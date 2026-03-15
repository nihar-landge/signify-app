package telephony

import (
	"context"
	"log"
	"sync"
	"time"
)

// BridgeSession represents a bridged call between WebRTC and PSTN
type BridgeSession struct {
	ID          string
	UserID      string
	CallSID     string
	PhoneNumber string
	State       BridgeState
	CreatedAt   time.Time
	ConnectedAt *time.Time
	EndedAt     *time.Time

	// Channels for data flow
	LandmarksIn chan []byte // Landmarks from Over app
	CaptionsOut chan string // Captions to Over app
	TTSAudioOut chan []byte // TTS audio to PSTN
	STTAudioIn  chan []byte // Audio from PSTN for STT

	AudioEncoding string // "mulaw" or "linear16"
	SampleRate    int    // 8000 or 16000

	// Cancel function
	cancel context.CancelFunc
}

// BridgeState represents the state of a bridge session
type BridgeState string

const (
	BridgeStateInitializing BridgeState = "initializing"
	BridgeStateDialing      BridgeState = "dialing"
	BridgeStateRinging      BridgeState = "ringing"
	BridgeStateConnected    BridgeState = "connected"
	BridgeStateEnding       BridgeState = "ending"
	BridgeStateEnded        BridgeState = "ended"
	BridgeStateFailed       BridgeState = "failed"
)

// Bridge manages the audio and data relay between PSTN and WebRTC
type Bridge struct {
	telephony TelephonyProvider
	mu        sync.RWMutex
	sessions  map[string]*BridgeSession // sessionID -> Session
	// Mapping from PSTN call SID to session
	sessionsByCall map[string]*BridgeSession

	// Callbacks for higher level API logic
	onStateChanged      func(sessionID string, state BridgeState)
	onLandmarksReceived func(sessionID string, landmarks []byte)
	onSTTAudioReceived  func(sessionID string, audio []byte)
}

// NewBridge creates a new audio/data bridge
func NewBridge(telephony TelephonyProvider) *Bridge {
	b := &Bridge{
		telephony:      telephony,
		sessions:       make(map[string]*BridgeSession),
		sessionsByCall: make(map[string]*BridgeSession),
	}

	// Register telephony callbacks
	telephony.SetCallAnsweredCallback(b.handleCallAnswered)
	telephony.SetCallEndedCallback(b.handleCallEnded)
	telephony.SetAudioReceivedCallback(b.handleAudioReceived)

	return b
}

// SetLandmarksCallback sets the handler for landmarks
func (b *Bridge) SetLandmarksCallback(cb func(sessionID string, landmarks []byte)) {
	b.onLandmarksReceived = cb
}

// SetSTTAudioCallback sets the handler for STT audio
func (b *Bridge) SetSTTAudioCallback(cb func(sessionID string, audio []byte)) {
	b.onSTTAudioReceived = cb
}

// SetStateChangedCallback sets the callback for state changes
func (b *Bridge) SetStateChangedCallback(cb func(sessionID string, state BridgeState)) {
	b.onStateChanged = cb
}

// CreateSession starts a new bridge session
func (b *Bridge) CreateSession(ctx context.Context, userID, phoneNumber string) (*BridgeSession, error) {
	sessionID := generateSessionID()

	sessionCtx, cancel := context.WithCancel(ctx)

	session := &BridgeSession{
		ID:          sessionID,
		UserID:      userID,
		PhoneNumber: phoneNumber,
		State:       BridgeStateInitializing,
		CreatedAt:   time.Now(),
		LandmarksIn: make(chan []byte, 50),
		CaptionsOut: make(chan string, 50),
		TTSAudioOut: make(chan []byte, 100),
		STTAudioIn:  make(chan []byte, 500),
		cancel:      cancel,
	}

	b.mu.Lock()
	b.sessions[sessionID] = session
	b.mu.Unlock()

	// Place PSTN call
	call, err := b.telephony.PlaceCall(sessionCtx, phoneNumber)
	if err != nil {
		b.updateSessionState(sessionID, BridgeStateFailed)
		return nil, err
	}

	session.CallSID = call.SID

	b.mu.Lock()
	b.sessionsByCall[call.SID] = session
	b.mu.Unlock()

	b.updateSessionState(sessionID, BridgeStateDialing)

	// Start processing goroutines
	go b.processLandmarks(sessionCtx, session)
	go b.processSTTAudio(sessionCtx, session)
	go b.processTTSAudio(sessionCtx, session, call)

	log.Printf("Created bridge session %s for user %s -> %s", sessionID, userID, phoneNumber)
	return session, nil
}

// RegisterInboundSession manually registers an inbound call session
func (b *Bridge) RegisterInboundSession(ctx context.Context, callSID, phoneNumber string) (*BridgeSession, error) {
	sessionID := generateSessionID()
	sessionCtx, cancel := context.WithCancel(ctx)

	session := &BridgeSession{
		ID:          sessionID,
		UserID:      "inbound_pending", // Will be updated when a client connects
		CallSID:     callSID,
		PhoneNumber: phoneNumber,
		State:       BridgeStateRinging,
		CreatedAt:   time.Now(),
		LandmarksIn: make(chan []byte, 50),
		CaptionsOut: make(chan string, 50),
		TTSAudioOut: make(chan []byte, 100),
		STTAudioIn:  make(chan []byte, 500),
		cancel:      cancel,
	}

	b.mu.Lock()
	b.sessions[sessionID] = session
	b.sessionsByCall[callSID] = session
	b.mu.Unlock()

	// Start processing goroutines for STT and TTS early (so we can play ringing if needed)
	go b.processLandmarks(sessionCtx, session)
	go b.processSTTAudio(sessionCtx, session)
	// We don't have the *Call object yet, we'll need to handle that when the connect happens?
	// Actually Twilio audio stream connects via Websocket to twilio.go-server

	log.Printf("Registered inbound bridge session %s for call %s from %s", sessionID, callSID, phoneNumber)
	return session, nil
}

// EndSession terminates a bridge session
func (b *Bridge) EndSession(ctx context.Context, sessionID string) error {
	b.mu.RLock()
	session, exists := b.sessions[sessionID]
	b.mu.RUnlock()

	if !exists {
		return nil
	}

	b.updateSessionState(sessionID, BridgeStateEnding)

	// Cancel the session context
	if session.cancel != nil {
		session.cancel()
	}

	// End the PSTN call
	if session.CallSID != "" {
		if err := b.telephony.EndCall(ctx, session.CallSID); err != nil {
			log.Printf("Error ending call: %v", err)
		}
	}

	// Clean up channels
	close(session.LandmarksIn)
	close(session.CaptionsOut)
	close(session.TTSAudioOut)
	close(session.STTAudioIn)

	now := time.Now()
	session.EndedAt = &now

	b.updateSessionState(sessionID, BridgeStateEnded)

	// Remove from maps
	b.mu.Lock()
	delete(b.sessions, sessionID)
	delete(b.sessionsByCall, session.CallSID)
	b.mu.Unlock()

	log.Printf("Ended bridge session %s", sessionID)
	return nil
}

// SendLandmarks sends landmark data from the Over app
func (b *Bridge) SendLandmarks(sessionID string, landmarks []byte) error {
	b.mu.RLock()
	session, exists := b.sessions[sessionID]
	b.mu.RUnlock()

	if !exists {
		return nil
	}

	select {
	case session.LandmarksIn <- landmarks:
	default:
		// Drop if buffer full
	}

	return nil
}

// SendCaption sends a caption to the Over app
func (b *Bridge) SendCaption(sessionID, caption string) error {
	b.mu.RLock()
	session, exists := b.sessions[sessionID]
	b.mu.RUnlock()

	if !exists {
		return nil
	}

	select {
	case session.CaptionsOut <- caption:
	default:
		// Drop if buffer full
	}

	return nil
}

// SendTTSAudio queues TTS audio to be sent to the PSTN call
func (b *Bridge) SendTTSAudio(sessionID string, audio []byte) error {
	b.mu.RLock()
	session, exists := b.sessions[sessionID]
	b.mu.RUnlock()

	if !exists {
		return nil
	}

	select {
	case session.TTSAudioOut <- audio:
	default:
		// Drop if buffer full
	}

	return nil
}

// GetSession returns a session by ID
func (b *Bridge) GetSession(sessionID string) (*BridgeSession, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	session, exists := b.sessions[sessionID]
	return session, exists
}

// GetSessionByUser returns a session by user ID
func (b *Bridge) GetSessionByUser(userID string) (*BridgeSession, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, session := range b.sessions {
		if session.UserID == userID {
			return session, true
		}
	}
	return nil, false
}

// GetCaptionsChannel returns the captions channel for a session
func (b *Bridge) GetCaptionsChannel(sessionID string) (<-chan string, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	session, exists := b.sessions[sessionID]
	if !exists {
		return nil, false
	}
	return session.CaptionsOut, true
}

// handleCallAnswered is called when the PSTN call is answered
func (b *Bridge) handleCallAnswered(callSID string) {
	b.mu.RLock()
	session, exists := b.sessionsByCall[callSID]
	b.mu.RUnlock()

	if !exists {
		return
	}

	now := time.Now()
	session.ConnectedAt = &now
	b.updateSessionState(session.ID, BridgeStateConnected)
}

// handleCallEnded is called when the PSTN call ends
func (b *Bridge) handleCallEnded(callSID string, duration int) {
	b.mu.RLock()
	session, exists := b.sessionsByCall[callSID]
	b.mu.RUnlock()

	if !exists {
		return
	}

	log.Printf("Call ended: %s, duration: %d seconds", callSID, duration)
	b.EndSession(context.Background(), session.ID)
}

// handleAudioReceived is called when audio is received from PSTN
func (b *Bridge) handleAudioReceived(callSID string, audio []byte) {
	b.mu.RLock()
	session, exists := b.sessionsByCall[callSID]
	b.mu.RUnlock()

	if !exists {
		return
	}

	select {
	case session.STTAudioIn <- audio:
	default:
		// Drop if buffer full
	}
}

// processLandmarks handles incoming landmark data
func (b *Bridge) processLandmarks(ctx context.Context, session *BridgeSession) {
	for {
		select {
		case <-ctx.Done():
			return
		case landmarks, ok := <-session.LandmarksIn:
			if !ok {
				return
			}
			if b.onLandmarksReceived != nil {
				b.onLandmarksReceived(session.ID, landmarks)
			}
		}
	}
}

// processSTTAudio handles audio from PSTN for STT
func (b *Bridge) processSTTAudio(ctx context.Context, session *BridgeSession) {
	for {
		select {
		case <-ctx.Done():
			return
		case audio, ok := <-session.STTAudioIn:
			if !ok {
				return
			}
			if b.onSTTAudioReceived != nil {
				b.onSTTAudioReceived(session.ID, audio)
			}
		}
	}
}

// processTTSAudio handles TTS audio to send to PSTN
func (b *Bridge) processTTSAudio(ctx context.Context, session *BridgeSession, call *Call) {
	for {
		select {
		case <-ctx.Done():
			return
		case audio, ok := <-session.TTSAudioOut:
			if !ok {
				return
			}
			// Send to Twilio call
			select {
			case call.AudioStream <- audio:
			default:
				// Drop if buffer full
			}
		}
	}
}

// updateSessionState updates the state of a session
func (b *Bridge) updateSessionState(sessionID string, state BridgeState) {
	b.mu.Lock()
	if session, exists := b.sessions[sessionID]; exists {
		session.State = state
	}
	b.mu.Unlock()

	if b.onStateChanged != nil {
		b.onStateChanged(sessionID, state)
	}

	log.Printf("Session %s state: %s", sessionID, state)
}

// generateSessionID creates a unique session ID
func generateSessionID() string {
	return time.Now().Format("20060102150405") + "_" + randomString(8)
}

// randomString generates a random alphanumeric string
func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		time.Sleep(1 * time.Nanosecond) // Ensure different values
	}
	return string(result)
}
