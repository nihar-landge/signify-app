package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// STTService manages Deepgram streaming STT connections
type STTService struct {
	apiKey string
	mu     sync.RWMutex
}

// STTSession represents an active STT session with Deepgram
type STTSession struct {
	sessionID   string
	conn        *websocket.Conn
	onPartial   func(text string, timestampMs int64)
	onFinal     func(text string, timestampMs int64)
	onError     func(err error)
	stopChan    chan struct{}
	doneChan    chan struct{}
	mu          sync.Mutex
	lastPartial string // For debouncing
}

// DeepgramMessage represents a message from Deepgram
type DeepgramMessage struct {
	Type    string `json:"type"`
	Channel struct {
		Alternatives []struct {
			Transcript string  `json:"transcript"`
			Confidence float64 `json:"confidence"`
		} `json:"alternatives"`
	} `json:"channel"`
	IsFinal     bool    `json:"is_final"`
	SpeechFinal bool    `json:"speech_final"`
	Duration    float64 `json:"duration"`
	Start       float64 `json:"start"`
}

// NewSTTService creates a new STT service
func NewSTTService(apiKey string) *STTService {
	return &STTService{
		apiKey: apiKey,
	}
}

// IsConfigured returns true if API key is set
func (s *STTService) IsConfigured() bool {
	return s.apiKey != ""
}

// StartSession creates a new Deepgram streaming session
// Callbacks are called from a separate goroutine
func (s *STTService) StartSession(
	sessionID string,
	encoding string,
	sampleRate int,
	language string,
	onPartial func(text string, timestampMs int64),
	onFinal func(text string, timestampMs int64),
	onError func(err error),
) (*STTSession, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("Deepgram API key not configured")
	}

	// Fallback to English if no language specified
	if language == "" {
		language = "en-US"
	}

	// Build Deepgram WebSocket URL with parameters
	params := url.Values{}
	params.Set("encoding", encoding)
	params.Set("sample_rate", fmt.Sprintf("%d", sampleRate))
	params.Set("channels", "1")
	params.Set("interim_results", "true") // Enable partial results
	params.Set("punctuate", "true")
	params.Set("language", language)
	params.Set("model", "nova-2") // Latest model

	wsURL := fmt.Sprintf("wss://api.deepgram.com/v1/listen?%s", params.Encode())

	// Create WebSocket connection with auth header
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	headers := map[string][]string{
		"Authorization": {fmt.Sprintf("Token %s", s.apiKey)},
	}

	conn, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Deepgram: %w", err)
	}

	session := &STTSession{
		sessionID: sessionID,
		conn:      conn,
		onPartial: onPartial,
		onFinal:   onFinal,
		onError:   onError,
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	// Start message reader goroutine
	go session.readMessages()

	log.Printf("STT session started: sessionId=%s", sessionID)
	return session, nil
}

// readMessages reads and processes messages from Deepgram
func (ss *STTSession) readMessages() {
	defer close(ss.doneChan)
	defer ss.conn.Close()

	for {
		select {
		case <-ss.stopChan:
			return
		default:
		}

		// Set read deadline to detect disconnection
		ss.conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		_, message, err := ss.conn.ReadMessage()
		if err != nil {
			select {
			case <-ss.stopChan:
				// Expected close
				return
			default:
				if ss.onError != nil {
					ss.onError(err)
				}
				return
			}
		}

		ss.processMessage(message)
	}
}

// processMessage parses and handles a Deepgram message
func (ss *STTSession) processMessage(data []byte) {
	var msg DeepgramMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("STT: failed to parse Deepgram message: %v", err)
		return
	}

	// Skip non-results messages
	if msg.Type != "Results" && msg.Type != "" {
		return
	}

	// Extract transcript from first alternative
	if len(msg.Channel.Alternatives) == 0 {
		return
	}

	transcript := msg.Channel.Alternatives[0].Transcript
	if transcript == "" {
		return
	}

	timestampMs := time.Now().UnixMilli()

	if msg.IsFinal || msg.SpeechFinal {
		// Final result
		if ss.onFinal != nil {
			ss.onFinal(transcript, timestampMs)
		}
		// Reset last partial
		ss.mu.Lock()
		ss.lastPartial = ""
		ss.mu.Unlock()
	} else {
		// Partial result - debounce duplicates
		ss.mu.Lock()
		if transcript != ss.lastPartial {
			ss.lastPartial = transcript
			ss.mu.Unlock()
			if ss.onPartial != nil {
				ss.onPartial(transcript, timestampMs)
			}
		} else {
			ss.mu.Unlock()
		}
	}
}

// PushAudio sends audio data to Deepgram
// Audio should be mulaw 8kHz (raw bytes, not base64)
func (ss *STTSession) PushAudio(audio []byte) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.conn == nil {
		return fmt.Errorf("STT session closed")
	}

	return ss.conn.WriteMessage(websocket.BinaryMessage, audio)
}

// Stop closes the STT session
func (ss *STTSession) Stop() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	select {
	case <-ss.stopChan:
		// Already stopped
		return
	default:
		close(ss.stopChan)
	}

	if ss.conn != nil {
		// Send close frame
		ss.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		ss.conn.Close()
		ss.conn = nil
	}

	log.Printf("STT session stopped: sessionId=%s", ss.sessionID)
}

// Wait waits for the STT session to fully close
func (ss *STTSession) Wait() {
	<-ss.doneChan
}
