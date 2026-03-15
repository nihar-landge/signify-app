package telephony

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ExotelConfig holds Exotel credentials and settings
type ExotelConfig struct {
	AccountSID    string
	APIKey        string
	APIToken      string
	PublicBaseURL string
	PublicWSURL   string
}

// ExotelClient handles PSTN calls via Exotel (India)
type ExotelClient struct {
	config      ExotelConfig
	httpClient  *http.Client
	activeCalls map[string]*Call
	mu          sync.RWMutex

	// Callbacks
	onCallAnswered  func(callSID string)
	onCallEnded     func(callSID string, duration int)
	onAudioReceived func(callSID string, audio []byte)
}

// NewExotelClient creates a new Exotel client
func NewExotelClient() *ExotelClient {
	config := ExotelConfig{
		AccountSID:    os.Getenv("EXOTEL_SID"),
		APIKey:        os.Getenv("EXOTEL_KEY"),
		APIToken:      os.Getenv("EXOTEL_TOKEN"),
		PublicBaseURL: os.Getenv("PUBLIC_BASE_URL"),
		PublicWSURL:   os.Getenv("PUBLIC_WS_URL"),
	}

	return &ExotelClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		activeCalls: make(map[string]*Call),
	}
}

// Ensure ExotelClient implements TelephonyProvider
var _ TelephonyProvider = (*ExotelClient)(nil)

func (ec *ExotelClient) SetCallAnsweredCallback(cb func(callSID string)) {
	ec.onCallAnswered = cb
}

func (ec *ExotelClient) SetCallEndedCallback(cb func(callSID string, duration int)) {
	ec.onCallEnded = cb
}

func (ec *ExotelClient) SetAudioReceivedCallback(cb func(callSID string, audio []byte)) {
	ec.onAudioReceived = cb
}

func (ec *ExotelClient) PlaceCall(ctx context.Context, toNumber string) (*Call, error) {
	// Exotel Connect API requires a 'From' (Exotel virtual number) and 'To' (Customer number)
	// Usually, for Signify, we want the call to connect to a Flow or a URL that handles the media stream.
	// We use the 'Url' parameter or 'FlowId'.

	// For this implementation, we'll use the newer connect.json endpoint which is more flexible for voicebots.
	connectURL := fmt.Sprintf("https://api.exotel.com/v1/Accounts/%s/Calls/connect.json", ec.config.AccountSID)

	// Since we don't have a fixed 'From' number in env yet, we look it up or use a placeholder.
	// In production, this would be your ExoPhone.
	fromNumber := os.Getenv("EXOTEL_FROM_NUMBER")
	if fromNumber == "" {
		return nil, fmt.Errorf("EXOTEL_FROM_NUMBER not configured")
	}

	// For streaming/voicebot, we need to point Exotel to our incoming webhook
	webhookURL := fmt.Sprintf("%s/exotel/incoming", ec.config.PublicBaseURL)

	// CallType=trans = transactional (required for voicebot/streaming calls in India)
	form := "From=" + fromNumber + "&To=" + toNumber + "&Url=" + webhookURL + "&CallType=trans"

	req, err := http.NewRequestWithContext(ctx, "POST", connectURL, strings.NewReader(form))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(ec.config.APIKey, ec.config.APIToken)

	resp, err := ec.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("exotel API error: %d", resp.StatusCode)
	}

	// Parse response to get Call SID
	var result struct {
		Call struct {
			Sid string `json:"Sid"`
		} `json:"Call"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse exotel response: %v", err)
	}

	call := &Call{
		SID:           result.Call.Sid,
		To:            toNumber,
		Status:        CallStatusRinging,
		StartTime:     time.Now(),
		AudioStream:   make(chan []byte, 100),
		AudioReceiver: make(chan []byte, 500),
	}

	ec.mu.Lock()
	ec.activeCalls[call.SID] = call
	ec.mu.Unlock()

	return call, nil
}

func (ec *ExotelClient) PlaceCallWithSession(ctx context.Context, sessionID, toNumber string) (*Call, error) {
	// Similar to PlaceCall, but we can pass the sessionID in the webhook URL
	connectURL := fmt.Sprintf("https://api.exotel.com/v1/Accounts/%s/Calls/connect.json", ec.config.AccountSID)
	fromNumber := os.Getenv("EXOTEL_FROM_NUMBER")
	if fromNumber == "" {
		return nil, fmt.Errorf("EXOTEL_FROM_NUMBER not configured")
	}

	webhookURL := fmt.Sprintf("%s/exotel/incoming?sessionId=%s", ec.config.PublicBaseURL, sessionID)

	// CallType=trans = transactional (required for voicebot/streaming calls in India)
	form := "From=" + fromNumber + "&To=" + toNumber + "&Url=" + webhookURL + "&CallType=trans"

	req, err := http.NewRequestWithContext(ctx, "POST", connectURL, strings.NewReader(form))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(ec.config.APIKey, ec.config.APIToken)

	resp, err := ec.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("exotel API error: %d", resp.StatusCode)
	}

	var result struct {
		Call struct {
			Sid string `json:"Sid"`
		} `json:"Call"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse exotel response: %v", err)
	}

	call := &Call{
		SID:           result.Call.Sid,
		To:            toNumber,
		Status:        CallStatusRinging,
		StartTime:     time.Now(),
		AudioStream:   make(chan []byte, 100),
		AudioReceiver: make(chan []byte, 500),
	}

	ec.mu.Lock()
	ec.activeCalls[call.SID] = call
	ec.mu.Unlock()

	return call, nil
}

func (ec *ExotelClient) EndCall(ctx context.Context, callSID string) error {
	ec.mu.Lock()
	_, exists := ec.activeCalls[callSID]
	if !exists {
		ec.mu.Unlock()
		return fmt.Errorf("call not found: %s", callSID)
	}
	ec.mu.Unlock()

	// Exotel hangup API
	hangupURL := fmt.Sprintf("https://api.exotel.com/v1/Accounts/%s/Calls/%s.json", ec.config.AccountSID, callSID)

	req, _ := http.NewRequestWithContext(ctx, "DELETE", hangupURL, nil)
	req.SetBasicAuth(ec.config.APIKey, ec.config.APIToken)

	resp, err := ec.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("exotel API error: %d", resp.StatusCode)
	}

	ec.mu.Lock()
	delete(ec.activeCalls, callSID)
	ec.mu.Unlock()

	return nil
}

func (ec *ExotelClient) GetCall(callSID string) (*Call, bool) {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	call, exists := ec.activeCalls[callSID]
	return call, exists
}

func (ec *ExotelClient) GetActiveCalls() []*Call {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	calls := make([]*Call, 0, len(ec.activeCalls))
	for _, c := range ec.activeCalls {
		calls = append(calls, c)
	}
	return calls
}

func (ec *ExotelClient) InjectAudio(callSID string, audioData []byte) error {
	ec.mu.RLock()
	call, exists := ec.activeCalls[callSID]
	ec.mu.RUnlock()

	if !exists {
		return fmt.Errorf("call not found")
	}

	select {
	case call.AudioStream <- audioData:
		return nil
	default:
		return fmt.Errorf("buffer full")
	}
}

func (ec *ExotelClient) HandleAudioStream(callSID string, audioData []byte) {
	ec.mu.RLock()
	call, exists := ec.activeCalls[callSID]
	ec.mu.RUnlock()

	if !exists {
		return
	}

	select {
	case call.AudioReceiver <- audioData:
	default:
	}

	if ec.onAudioReceived != nil {
		ec.onAudioReceived(callSID, audioData)
	}
}

func (ec *ExotelClient) HandleCallStatusWebhook(callSID string, status string) {
	// Exotel status mapping
	ec.mu.Lock()
	if call, exists := ec.activeCalls[callSID]; exists {
		call.Status = CallStatus(status)
	}
	ec.mu.Unlock()
}
