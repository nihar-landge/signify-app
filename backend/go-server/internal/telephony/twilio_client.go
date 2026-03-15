package telephony

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// TwilioConfig holds Twilio credentials and settings
type TwilioConfig struct {
	AccountSID    string
	AuthToken     string
	PhoneNumber   string // Your Twilio phone number
	PublicBaseURL string // Public HTTPS URL for webhooks
	PublicWSURL   string // Public WSS URL for WebSocket streams
}

// CallStatus represents the status of a phone call
type CallStatus string

const (
	CallStatusQueued     CallStatus = "queued"
	CallStatusRinging    CallStatus = "ringing"
	CallStatusInProgress CallStatus = "in-progress"
	CallStatusCompleted  CallStatus = "completed"
	CallStatusBusy       CallStatus = "busy"
	CallStatusFailed     CallStatus = "failed"
	CallStatusNoAnswer   CallStatus = "no-answer"
	CallStatusCanceled   CallStatus = "canceled"
)

// Call represents an active phone call
type Call struct {
	SID           string
	To            string
	From          string
	Status        CallStatus
	StartTime     time.Time
	Duration      int
	AudioStream   chan []byte // Audio from PSTN to inject TTS
	AudioReceiver chan []byte // Audio from PSTN for STT
}

// TwilioClient handles PSTN calls via Twilio
type TwilioClient struct {
	config      TwilioConfig
	httpClient  *http.Client
	activeCalls map[string]*Call
	mu          sync.RWMutex

	// Callbacks
	onCallAnswered  func(callSID string)
	onCallEnded     func(callSID string, duration int)
	onAudioReceived func(callSID string, audio []byte)
}

// NewTwilioClient creates a new Twilio client
func NewTwilioClient() *TwilioClient {
	config := TwilioConfig{
		AccountSID:    os.Getenv("TWILIO_ACCOUNT_SID"),
		AuthToken:     os.Getenv("TWILIO_AUTH_TOKEN"),
		PhoneNumber:   getEnvWithFallback("TWILIO_FROM_NUMBER", "TWILIO_PHONE_NUMBER"),
		PublicBaseURL: os.Getenv("PUBLIC_BASE_URL"),
		PublicWSURL:   os.Getenv("PUBLIC_WS_URL"),
	}

	if config.AccountSID == "" || config.AuthToken == "" {
		log.Println("Warning: Twilio credentials not configured. PSTN calls will be simulated.")
	}

	return &TwilioClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		activeCalls: make(map[string]*Call),
	}
}

// getEnvWithFallback tries primary key first, then fallback key
func getEnvWithFallback(primary, fallbackKey string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	return os.Getenv(fallbackKey)
}

// SetCallAnsweredCallback sets the callback for when a call is answered
func (tc *TwilioClient) SetCallAnsweredCallback(cb func(callSID string)) {
	tc.onCallAnswered = cb
}

// SetCallEndedCallback sets the callback for when a call ends
func (tc *TwilioClient) SetCallEndedCallback(cb func(callSID string, duration int)) {
	tc.onCallEnded = cb
}

// SetAudioReceivedCallback sets the callback for incoming audio
func (tc *TwilioClient) SetAudioReceivedCallback(cb func(callSID string, audio []byte)) {
	tc.onAudioReceived = cb
}

// PlaceCall initiates an outbound call (legacy method, uses inline TwiML)
func (tc *TwilioClient) PlaceCall(ctx context.Context, toNumber string) (*Call, error) {
	// Validate phone number format
	if !strings.HasPrefix(toNumber, "+") {
		toNumber = "+1" + strings.TrimPrefix(toNumber, "1") // Default to US
	}

	// If no Twilio credentials, simulate call
	if tc.config.AccountSID == "" {
		return tc.simulateCall(toNumber)
	}

	// Make actual Twilio API call
	callURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls.json", tc.config.AccountSID)

	// TwiML for the call - uses <Stream> for real-time audio
	twiml := fmt.Sprintf(`
		<Response>
			<Connect>
				<Stream url="%s/twilio/audio-stream" />
			</Connect>
		</Response>
	`, tc.config.PublicWSURL)

	data := url.Values{}
	data.Set("To", toNumber)
	data.Set("From", tc.config.PhoneNumber)
	data.Set("Twiml", twiml)
	data.Set("StatusCallback", tc.config.PublicBaseURL+"/twilio/status")
	data.Set("StatusCallbackEvent", "initiated ringing answered completed")

	req, err := http.NewRequestWithContext(ctx, "POST", callURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(tc.config.AccountSID, tc.config.AuthToken)

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to place call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("twilio API error: status %d", resp.StatusCode)
	}

	var result struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	call := &Call{
		SID:           result.SID,
		To:            toNumber,
		From:          tc.config.PhoneNumber,
		Status:        CallStatus(result.Status),
		StartTime:     time.Now(),
		AudioStream:   make(chan []byte, 100),
		AudioReceiver: make(chan []byte, 100),
	}

	tc.mu.Lock()
	tc.activeCalls[call.SID] = call
	tc.mu.Unlock()

	log.Printf("Placed call to %s, SID: %s", toNumber, call.SID)
	return call, nil
}

// PlaceCallWithSession initiates an outbound call using a TwiML URL with sessionId
func (tc *TwilioClient) PlaceCallWithSession(ctx context.Context, sessionID, toNumber string) (*Call, error) {
	// Validate phone number format
	if !strings.HasPrefix(toNumber, "+") {
		toNumber = "+1" + strings.TrimPrefix(toNumber, "1") // Default to US
	}

	// If no Twilio credentials, simulate call
	if tc.config.AccountSID == "" {
		return tc.simulateCall(toNumber)
	}

	// Make actual Twilio API call
	callURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls.json", tc.config.AccountSID)

	// Use URL-based TwiML instead of inline TwiML
	twimlURL := fmt.Sprintf("%s/twilio/twiml?sessionId=%s", tc.config.PublicBaseURL, url.QueryEscape(sessionID))

	data := url.Values{}
	data.Set("To", toNumber)
	data.Set("From", tc.config.PhoneNumber)
	data.Set("Url", twimlURL)
	data.Set("StatusCallback", fmt.Sprintf("%s/twilio/status?sessionId=%s", tc.config.PublicBaseURL, url.QueryEscape(sessionID)))
	data.Set("StatusCallbackMethod", "POST")
	// Use individual StatusCallbackEvent entries
	data.Add("StatusCallbackEvent", "initiated")
	data.Add("StatusCallbackEvent", "ringing")
	data.Add("StatusCallbackEvent", "answered")
	data.Add("StatusCallbackEvent", "completed")

	req, err := http.NewRequestWithContext(ctx, "POST", callURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(tc.config.AccountSID, tc.config.AuthToken)

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to place call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("twilio API error: status %d", resp.StatusCode)
	}

	var result struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	call := &Call{
		SID:           result.SID,
		To:            toNumber,
		From:          tc.config.PhoneNumber,
		Status:        CallStatus(result.Status),
		StartTime:     time.Now(),
		AudioStream:   make(chan []byte, 100),
		AudioReceiver: make(chan []byte, 100),
	}

	tc.mu.Lock()
	tc.activeCalls[call.SID] = call
	tc.mu.Unlock()

	log.Printf("Placed call with session to %s, SID: %s, SessionID: %s", toNumber, call.SID, sessionID)
	return call, nil
}

// GetConfig returns the Twilio configuration (for building TwiML responses)
func (tc *TwilioClient) GetConfig() TwilioConfig {
	return tc.config
}

// simulateCall creates a simulated call for testing without Twilio
func (tc *TwilioClient) simulateCall(toNumber string) (*Call, error) {
	callSID := fmt.Sprintf("SIMULATED_%d", time.Now().UnixNano())

	call := &Call{
		SID:           callSID,
		To:            toNumber,
		From:          "+15551234567",
		Status:        CallStatusQueued,
		StartTime:     time.Now(),
		AudioStream:   make(chan []byte, 100),
		AudioReceiver: make(chan []byte, 100),
	}

	tc.mu.Lock()
	tc.activeCalls[call.SID] = call
	tc.mu.Unlock()

	// Simulate call progression
	go func() {
		time.Sleep(1 * time.Second)
		tc.updateCallStatus(callSID, CallStatusRinging)

		time.Sleep(2 * time.Second)
		tc.updateCallStatus(callSID, CallStatusInProgress)

		if tc.onCallAnswered != nil {
			tc.onCallAnswered(callSID)
		}
	}()

	log.Printf("Simulated call to %s, SID: %s", toNumber, callSID)
	return call, nil
}

// EndCall terminates an active call
func (tc *TwilioClient) EndCall(ctx context.Context, callSID string) error {
	tc.mu.Lock()
	call, exists := tc.activeCalls[callSID]
	if !exists {
		tc.mu.Unlock()
		return fmt.Errorf("call not found: %s", callSID)
	}
	tc.mu.Unlock()

	// If simulated call, just end it
	if strings.HasPrefix(callSID, "SIMULATED_") {
		tc.updateCallStatus(callSID, CallStatusCompleted)
		return nil
	}

	// Make Twilio API call to end the call
	endURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls/%s.json",
		tc.config.AccountSID, callSID)

	data := url.Values{}
	data.Set("Status", "completed")

	req, err := http.NewRequestWithContext(ctx, "POST", endURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(tc.config.AccountSID, tc.config.AuthToken)

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to end call: %w", err)
	}
	defer resp.Body.Close()

	// Clean up
	close(call.AudioStream)
	close(call.AudioReceiver)

	tc.mu.Lock()
	delete(tc.activeCalls, callSID)
	tc.mu.Unlock()

	if tc.onCallEnded != nil {
		duration := int(time.Since(call.StartTime).Seconds())
		tc.onCallEnded(callSID, duration)
	}

	log.Printf("Ended call SID: %s", callSID)
	return nil
}

// InjectAudio sends TTS audio to the PSTN call
func (tc *TwilioClient) InjectAudio(callSID string, audioData []byte) error {
	tc.mu.RLock()
	call, exists := tc.activeCalls[callSID]
	tc.mu.RUnlock()

	if !exists {
		return fmt.Errorf("call not found: %s", callSID)
	}

	select {
	case call.AudioStream <- audioData:
		return nil
	default:
		return fmt.Errorf("audio buffer full")
	}
}

// GetCall returns an active call by SID
func (tc *TwilioClient) GetCall(callSID string) (*Call, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	call, exists := tc.activeCalls[callSID]
	return call, exists
}

// HandleCallStatusWebhook processes Twilio status callbacks
func (tc *TwilioClient) HandleCallStatusWebhook(callSID string, status string) {
	tc.updateCallStatus(callSID, CallStatus(status))

	switch CallStatus(status) {
	case CallStatusInProgress:
		if tc.onCallAnswered != nil {
			tc.onCallAnswered(callSID)
		}
	case CallStatusCompleted, CallStatusFailed, CallStatusBusy, CallStatusNoAnswer:
		tc.mu.Lock()
		call, exists := tc.activeCalls[callSID]
		if exists {
			close(call.AudioStream)
			close(call.AudioReceiver)
			delete(tc.activeCalls, callSID)
		}
		tc.mu.Unlock()

		if tc.onCallEnded != nil {
			duration := 0
			if exists {
				duration = int(time.Since(call.StartTime).Seconds())
			}
			tc.onCallEnded(callSID, duration)
		}
	}
}

// HandleAudioStream processes incoming audio from Twilio Media Stream
func (tc *TwilioClient) HandleAudioStream(callSID string, audioData []byte) {
	tc.mu.RLock()
	call, exists := tc.activeCalls[callSID]
	tc.mu.RUnlock()

	if !exists {
		return
	}

	select {
	case call.AudioReceiver <- audioData:
	default:
		// Drop audio if buffer is full
	}

	if tc.onAudioReceived != nil {
		tc.onAudioReceived(callSID, audioData)
	}
}

// updateCallStatus updates the status of a call
func (tc *TwilioClient) updateCallStatus(callSID string, status CallStatus) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if call, exists := tc.activeCalls[callSID]; exists {
		call.Status = status
		log.Printf("Call %s status: %s", callSID, status)
	}
}

// GetActiveCalls returns a list of all active calls
func (tc *TwilioClient) GetActiveCalls() []*Call {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	calls := make([]*Call, 0, len(tc.activeCalls))
	for _, call := range tc.activeCalls {
		calls = append(calls, call)
	}
	return calls
}
