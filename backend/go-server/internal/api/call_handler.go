package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"signify/internal/model"
	"signify/internal/repository"
	"signify/internal/service"
	"signify/internal/telephony"
	"signify/internal/webrtc"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	fiberwebsocket "github.com/gofiber/websocket/v2"
	"signify/pkg/config"
)

// AppEvent types for WebSocket messages to Over app
type AppEvent struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Status    string `json:"status,omitempty"`
	Message   string `json:"message,omitempty"`
	Audio     string `json:"audio,omitempty"` // Base64 encoded audio for testing
	Timestamp int64  `json:"timestamp,omitempty"`
}

// CallHandler manages relay call functionality
type CallHandler struct {
	signalingHub      *webrtc.SignalingHub
	bridge            *telephony.Bridge
	telephony         telephony.TelephonyProvider
	landmarkProcessor *service.LandmarkProcessor
	sttProcessor      *service.StreamProcessor
	sttService        *service.STTService
	inferenceClient   *service.InferenceClient
	stabilizer        *service.SignStabilizer
	sentenceBuilder   *service.MultiSessionSentenceBuilder
	sessionRepo       *repository.CallSessionRepository
	cfg               *config.Config

	// Active call sessions (legacy - for existing flow)
	activeSessions map[string]*CallSessionInfo
	mu             sync.RWMutex

	// Global connections for broadcasting events (e.g. incoming calls)
	GlobalAppConns map[*fiberwebsocket.Conn]bool
	GlobalMu       sync.Mutex
}

// CallSessionInfo tracks a call session
type CallSessionInfo struct {
	UserID      string
	BridgeID    string
	PhoneNumber string
	StartTime   time.Time

	// Channels for data flow
	TextOutput  chan service.TranslationResult
	AudioOutput chan []byte
	CaptionChan <-chan string

	// Context for cancellation
	Cancel context.CancelFunc
}

// NewCallHandler creates a new call handler
func NewCallHandler(landmarkProcessor *service.LandmarkProcessor, sttProcessor *service.StreamProcessor, inferenceClient *service.InferenceClient, cfg *config.Config) *CallHandler {
	// Create telephony provider based on config
	var telephonyProvider telephony.TelephonyProvider

	// Prefer Exotel if configured, otherwise fallback to Twilio
	if cfg.ExotelSID != "" && cfg.ExotelKey != "" && cfg.ExotelToken != "" {
		telephonyProvider = telephony.NewExotelClient()
		log.Println("Using Exotel as telephony provider")
	} else {
		telephonyProvider = telephony.NewTwilioClient()
		log.Println("Using Twilio as telephony provider")
	}

	bridge := telephony.NewBridge(telephonyProvider)

	// Create signaling hub
	signalingHub := webrtc.NewSignalingHub()

	// Create session repository
	sessionRepo := repository.NewCallSessionRepository()

	// Create STT service (Deepgram)
	sttService := service.NewSTTService(os.Getenv("DEEPGRAM_API_KEY"))

	// Create Stabilizer: Window=5, Min=3, Cooldown=1.5s
	stabilizer := service.NewSignStabilizer(5, 3, 1500*time.Millisecond)

	// Create Sentence Builder: Timeout=1.2s (More responsive for testing)
	sentenceBuilder := service.NewMultiSessionSentenceBuilder(1200 * time.Millisecond)

	handler := &CallHandler{
		signalingHub:      signalingHub,
		bridge:            bridge,
		telephony:         telephonyProvider,
		landmarkProcessor: landmarkProcessor,
		sttProcessor:      sttProcessor,
		sttService:        sttService,
		inferenceClient:   inferenceClient,
		stabilizer:        stabilizer,
		sentenceBuilder:   sentenceBuilder,
		sessionRepo:       sessionRepo,
		cfg:               cfg,
		activeSessions:    make(map[string]*CallSessionInfo),
		GlobalAppConns:    make(map[*fiberwebsocket.Conn]bool),
	}

	// Wire up callbacks
	signalingHub.SetCallInitiateHandler(handler.handleCallInitiate)
	signalingHub.SetCallAcceptHandler(handler.handleCallAccept)
	signalingHub.SetCallEndHandler(handler.handleCallEnd)
	signalingHub.SetLandmarksHandler(handler.handleLandmarks)

	bridge.SetStateChangedCallback(handler.handleBridgeStateChange)
	bridge.SetLandmarksCallback(handler.handleBridgeLandmarks)
	bridge.SetSTTAudioCallback(handler.handleSTTAudio)

	return handler
}

// HandleCallWebSocket handles WebSocket connections for calls
func (h *CallHandler) HandleCallWebSocket(c *fiberwebsocket.Conn) {
	userID := c.Params("userID")
	if userID == "" {
		log.Println("Missing userID in call WebSocket")
		c.Close()
		return
	}

	h.signalingHub.HandleConnection(c, userID)
}

// InitiateCall is the REST endpoint to initiate a call
func (h *CallHandler) InitiateCall(c *fiber.Ctx) error {
	type Request struct {
		PhoneNumber string `json:"phone_number"`
	}

	var req Request
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	userID := c.Params("userID")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing userID"})
	}

	bridgeID, err := h.handleCallInitiate(userID, req.PhoneNumber)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"bridge_id":    bridgeID,
		"phone_number": req.PhoneNumber,
		"status":       "dialing",
	})
}

// EndCall is the REST endpoint to end a call
func (h *CallHandler) EndCall(c *fiber.Ctx) error {
	userID := c.Params("userID")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing userID"})
	}

	h.handleCallEnd(userID)

	return c.JSON(fiber.Map{"status": "ended"})
}

// handleCallInitiate processes call initiation
func (h *CallHandler) handleCallInitiate(userID, phoneNumber string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create bridge session
	session, err := h.bridge.CreateSession(ctx, userID, phoneNumber)
	if err != nil {
		cancel()
		return "", err
	}

	// Create output channels
	textOutput := make(chan service.TranslationResult, 50)
	audioOutput := make(chan []byte, 100)

	// Get captions channel
	captionChan, _ := h.bridge.GetCaptionsChannel(session.ID)

	// Store session info
	sessionInfo := &CallSessionInfo{
		UserID:      userID,
		BridgeID:    session.ID,
		PhoneNumber: phoneNumber,
		StartTime:   time.Now(),
		TextOutput:  textOutput,
		AudioOutput: audioOutput,
		CaptionChan: captionChan,
		Cancel:      cancel,
	}

	h.mu.Lock()
	h.activeSessions[userID] = sessionInfo
	h.mu.Unlock()

	// Start landmark processor for this session
	h.landmarkProcessor.StartSession(session.ID)

	// Start processing goroutines
	go h.processTranslationOutput(ctx, sessionInfo)
	go h.processCaptions(ctx, sessionInfo)
	go h.landmarkProcessor.ProcessContinuous(ctx, session.ID, textOutput, audioOutput)

	return session.ID, nil
}

// handleCallAccept processes an incoming call acceptance from a user
func (h *CallHandler) handleCallAccept(userID, sessionID string) error {
	ctx, cancel := context.WithCancel(context.Background())

	// Retrieve the existing inbound bridge session
	session, exists := h.bridge.GetSession(sessionID)
	if !exists {
		cancel()
		return fmt.Errorf("inbound session %s not found", sessionID)
	}

	// Link user to the session
	session.UserID = userID

	// Create output channels
	textOutput := make(chan service.TranslationResult, 50)
	audioOutput := make(chan []byte, 100)

	// Get captions channel
	captionChan, _ := h.bridge.GetCaptionsChannel(session.ID)

	// Store session info
	sessionInfo := &CallSessionInfo{
		UserID:      userID,
		BridgeID:    session.ID,
		PhoneNumber: session.PhoneNumber,
		StartTime:   time.Now(),
		TextOutput:  textOutput,
		AudioOutput: audioOutput,
		CaptionChan: captionChan,
		Cancel:      cancel,
	}

	h.mu.Lock()
	h.activeSessions[userID] = sessionInfo
	h.mu.Unlock()

	// Start landmark processor for this session
	h.landmarkProcessor.StartSession(session.ID)

	// Start processing goroutines
	go h.processTranslationOutput(ctx, sessionInfo)
	go h.processCaptions(ctx, sessionInfo)
	go h.landmarkProcessor.ProcessContinuous(ctx, session.ID, textOutput, audioOutput)

	log.Printf("Inbound call session accepted and linked: user=%s, bridge=%s, phone=%s", userID, session.ID, session.PhoneNumber)

	return nil
}

// handleCallEnd processes call termination
func (h *CallHandler) handleCallEnd(userID string) {
	h.mu.Lock()
	sessionInfo, exists := h.activeSessions[userID]
	if exists {
		delete(h.activeSessions, userID)
	}
	h.mu.Unlock()

	if !exists {
		return
	}

	// Cancel the context
	if sessionInfo.Cancel != nil {
		sessionInfo.Cancel()
	}

	// End bridge session
	h.bridge.EndSession(context.Background(), sessionInfo.BridgeID)

	// End landmark processor session
	h.landmarkProcessor.EndSession(sessionInfo.BridgeID)

	// Notify client
	h.signalingHub.SendCallEnded(userID)

	log.Printf("Call session ended: user=%s", userID)
}

// handleLandmarks processes incoming landmark data
func (h *CallHandler) handleLandmarks(userID string, data []byte) {
	h.mu.RLock()
	sessionInfo, exists := h.activeSessions[userID]
	h.mu.RUnlock()

	if !exists {
		return
	}

	// Add to landmark processor
	h.landmarkProcessor.AddFrame(sessionInfo.BridgeID, data)

	// Also send to bridge
	h.bridge.SendLandmarks(sessionInfo.BridgeID, data)
}

// handleBridgeStateChange handles bridge state changes
func (h *CallHandler) handleBridgeStateChange(sessionID string, state telephony.BridgeState) {
	// Find the user for this session
	h.mu.RLock()
	var foundUserID string
	for userID, info := range h.activeSessions {
		if info.BridgeID == sessionID {
			foundUserID = userID
			break
		}
	}
	h.mu.RUnlock()

	if foundUserID == "" {
		return
	}

	switch state {
	case telephony.BridgeStateConnected:
		h.signalingHub.SendCallConnected(foundUserID)
	case telephony.BridgeStateEnded, telephony.BridgeStateFailed:
		h.signalingHub.SendCallEnded(foundUserID)
	}
}

// handleBridgeLandmarks handles landmarks from the bridge (forwarded)
func (h *CallHandler) handleBridgeLandmarks(sessionID string, landmarks []byte) {
	// Landmarks are already being processed via the direct path
}

// handleSTTAudio processes audio from PSTN for STT
func (h *CallHandler) handleSTTAudio(sessionID string, audio []byte) {
	// Find the legacy call session
	h.mu.RLock()
	var foundInfo *CallSessionInfo
	for _, info := range h.activeSessions {
		if info.BridgeID == sessionID {
			foundInfo = info
			break
		}
	}
	h.mu.RUnlock()

	if foundInfo == nil {
		return
	}

	// Try to push audio to the session-based STT pipeline
	// The startSTTWorker reads from session.AudioIn and forwards to Deepgram
	session, err := h.sessionRepo.GetSession(sessionID)
	if err == nil && session.AudioIn != nil {
		session.PushAudio(audio)
	}
}

// processTranslationOutput handles translation results
func (h *CallHandler) processTranslationOutput(ctx context.Context, sessionInfo *CallSessionInfo) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-sessionInfo.TextOutput:
			if !ok {
				return
			}

			// Send TTS audio to bridge
			if len(sessionInfo.AudioOutput) > 0 {
				select {
				case audio := <-sessionInfo.AudioOutput:
					h.bridge.SendTTSAudio(sessionInfo.BridgeID, audio)
				default:
				}
			}

			log.Printf("Translation result: %s (final=%v)", result.Text, result.IsFinal)
		case audio, ok := <-sessionInfo.AudioOutput:
			if !ok {
				return
			}

			// Send TTS audio to bridge (to PSTN)
			h.bridge.SendTTSAudio(sessionInfo.BridgeID, audio)
		}
	}
}

// processCaptions handles captions from STT
func (h *CallHandler) processCaptions(ctx context.Context, sessionInfo *CallSessionInfo) {
	if sessionInfo.CaptionChan == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case caption, ok := <-sessionInfo.CaptionChan:
			if !ok {
				return
			}

			// Send caption to client
			h.signalingHub.SendCaption(sessionInfo.UserID, caption)
		}
	}
}

// GetSignalingHub returns the signaling hub for route setup
func (h *CallHandler) GetSignalingHub() *webrtc.SignalingHub {
	return h.signalingHub
}

// GetBridge returns the bridge for external access
func (h *CallHandler) GetBridge() *telephony.Bridge {
	return h.bridge
}

// SendTestCaption sends a test caption (for debugging)
func (h *CallHandler) SendTestCaption(userID, text string) {
	h.mu.RLock()
	_, exists := h.activeSessions[userID]
	h.mu.RUnlock()

	if !exists {
		return
	}

	h.signalingHub.SendCaption(userID, text)
}

// callStats returns statistics about active calls
type CallStats struct {
	ActiveCalls int                `json:"active_calls"`
	Sessions    []CallSessionStats `json:"sessions"`
}

type CallSessionStats struct {
	UserID      string `json:"user_id"`
	PhoneNumber string `json:"phone_number"`
	Duration    int    `json:"duration_seconds"`
}

// GetStats returns call statistics
func (h *CallHandler) GetStats() CallStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats := CallStats{
		ActiveCalls: len(h.activeSessions),
		Sessions:    make([]CallSessionStats, 0, len(h.activeSessions)),
	}

	for _, info := range h.activeSessions {
		stats.Sessions = append(stats.Sessions, CallSessionStats{
			UserID:      info.UserID,
			PhoneNumber: info.PhoneNumber,
			Duration:    int(time.Since(info.StartTime).Seconds()),
		})
	}

	return stats
}

// Twilio webhook handlers

// HandleTwilioStatus handles Twilio call status webhooks
func (h *CallHandler) HandleTwilioStatus(c *fiber.Ctx) error {
	callSID := c.FormValue("CallSid")
	status := c.FormValue("CallStatus")
	sessionID := c.Query("sessionId")

	log.Printf("Twilio status webhook: CallSid=%s, Status=%s, SessionId=%s", callSID, status, sessionID)

	// Update session status if we have a sessionId
	if sessionID != "" {
		var sessionStatus model.CallSessionStatus
		switch status {
		case "ringing":
			sessionStatus = model.CallSessionStatusDialing
		case "in-progress":
			sessionStatus = model.CallSessionStatusAnswered
		case "completed", "busy", "failed", "no-answer", "canceled":
			sessionStatus = model.CallSessionStatusCompleted
			if status == "failed" {
				sessionStatus = model.CallSessionStatusFailed
			}
		}
		if sessionStatus != "" {
			_ = h.sessionRepo.SetStatus(sessionID, sessionStatus)
			if sessionStatus == model.CallSessionStatusCompleted || sessionStatus == model.CallSessionStatusFailed {
				if session, err := h.sessionRepo.GetSession(sessionID); err == nil {
					session.RequestSTTStop()
				}
			}
		}
	}

	// Forward to telephony client for call management
	h.telephony.HandleCallStatusWebhook(callSID, status)

	return c.SendString("OK")
}

// HandleTwilioAudioStream handles Twilio Media Stream WebSocket connections
// Events received: connected → start → media (repeating) → stop
// Audio format: audio/x-mulaw @ 8000 Hz, base64 encoded
func (h *CallHandler) HandleTwilioAudioStream(c *fiberwebsocket.Conn) {
	log.Println("Twilio audio stream: WebSocket connection established, waiting for start event...")

	var sessionID string
	var session *model.CallSession
	var streamSid string
	var frameCount int
	var droppedFrames int

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Outbound goroutine for Signal-to-Voice (Twilio)
	go func() {
		var bridgeSession *telephony.BridgeSession
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if sessionID == "" || streamSid == "" {
					time.Sleep(100 * time.Millisecond)
					continue
				}

				if bridgeSession == nil {
					b, exists := h.bridge.GetSession(sessionID)
					if exists {
						bridgeSession = b
					} else {
						time.Sleep(100 * time.Millisecond)
						continue
					}
				}

				// Wait for audio from TTS (Bridge handles the Sign-to-Voice pipeline)
				select {
				case <-ctx.Done():
					return
				case audio, ok := <-bridgeSession.TTSAudioOut:
					if !ok || audio == nil {
						if !ok {
							return
						}
						continue
					}
					// Wrap in Twilio JSON format (expects mulaw 8kHz)
					payload := base64.StdEncoding.EncodeToString(audio)
					msg := map[string]interface{}{
						"event":     "media",
						"streamSid": streamSid,
						"media": map[string]string{
							"payload": payload,
						},
					}
					jsonMsg, _ := json.Marshal(msg)
					if err := c.WriteMessage(fiberwebsocket.TextMessage, jsonMsg); err != nil {
						log.Printf("Failed to write to Twilio stream: %v", err)
						return
					}
				}
			}
		}
	}()

	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			if sessionID != "" {
				log.Printf("Twilio stream error for session %s: %v", sessionID, err)
			} else {
				log.Printf("Twilio stream error (pre-session): %v", err)
			}
			break
		}

		var streamMsg struct {
			Event     string `json:"event"`
			StreamSid string `json:"streamSid"`
			Start     struct {
				CallSid          string            `json:"callSid"`
				StreamSid        string            `json:"streamSid"`
				AccountSid       string            `json:"accountSid"`
				CustomParameters map[string]string `json:"customParameters"` // Contains our sessionId
				MediaFormat      struct {
					Encoding   string `json:"encoding"`   // audio/x-mulaw
					SampleRate int    `json:"sampleRate"` // 8000
					Channels   int    `json:"channels"`   // 1
				} `json:"mediaFormat"`
			} `json:"start"`
			Media struct {
				Track     string `json:"track"`     // inbound or outbound
				Chunk     string `json:"chunk"`     // sequence number
				Timestamp string `json:"timestamp"` // milliseconds
				Payload   string `json:"payload"`   // Base64 encoded audio (mulaw 8kHz)
			} `json:"media"`
		}

		if err := json.Unmarshal(msg, &streamMsg); err != nil {
			log.Printf("Failed to parse Twilio message: %v", err)
			continue
		}

		switch streamMsg.Event {
		case "connected":
			// First event - WebSocket connection established with Twilio
			log.Printf("Twilio stream event=connected")

		case "start":
			// Stream started - extract sessionId from customParameters
			sessionID = streamMsg.Start.CustomParameters["sessionId"]
			if sessionID == "" {
				log.Println("Twilio stream error: sessionId not found in customParameters, closing connection")
				c.Close()
				return
			}

			// Lookup session
			var err error
			session, err = h.sessionRepo.GetSession(sessionID)
			if err != nil {
				if h.cfg.AllowTestSessions && strings.HasPrefix(sessionID, "test-session-") {
					log.Printf("Twilio audio stream: Auto-creating test session: %s", sessionID)
					session = model.NewCallSession(sessionID, "+0000000000")
					h.sessionRepo.AddSession(session)
				} else {
					log.Printf("Twilio audio stream: session not found: %s", sessionID)
					c.Close()
					return
				}
			}

			streamSid = streamMsg.Start.StreamSid
			callSid := streamMsg.Start.CallSid

			// Save streamSid to session
			_ = h.sessionRepo.SetStreamSid(sessionID, streamSid)
			_ = h.sessionRepo.SetCallSid(sessionID, callSid)
			_ = h.sessionRepo.SetStatus(sessionID, model.CallSessionStatusStreaming)

			log.Printf("Twilio stream event=start: sessionId=%s, streamSid=%s, callSid=%s, encoding=%s, sampleRate=%d",
				sessionID, streamSid, callSid,
				streamMsg.Start.MediaFormat.Encoding,
				streamMsg.Start.MediaFormat.SampleRate)

		case "media":
			if session == nil {
				continue
			}

			if session.AudioIn == nil {
				continue
			}

			// Decode base64 audio payload (mulaw 8kHz)
			audioData, err := base64.StdEncoding.DecodeString(streamMsg.Media.Payload)
			if err != nil {
				log.Printf("Failed to decode audio for session %s: %v", sessionID, err)
				continue
			}

			// Push audio to session channel (non-blocking to prevent backpressure)
			if session.PushAudio(audioData) {
				// Start STT only after first media frame arrives.
				h.startSTTWorkerIfNeeded(session, "first_media")
				frameCount++
				if frameCount%100 == 0 {
					log.Printf("Session %s: received %d audio frames (track=%s)",
						sessionID, frameCount, streamMsg.Media.Track)
				}
			} else {
				droppedFrames++
				if droppedFrames%50 == 0 {
					log.Printf("Session %s: dropped %d frames due to buffer full", sessionID, droppedFrames)
				}
			}

		case "stop":
			log.Printf("Twilio stream event=stop: sessionId=%s, streamSid=%s, total frames=%d, dropped=%d",
				sessionID, streamSid, frameCount, droppedFrames)
			if session != nil {
				session.RequestSTTStop()
			}
			_ = h.sessionRepo.SetStatus(sessionID, model.CallSessionStatusCompleted)
			return
		}
	}

	// Cleanup on disconnect
	if session != nil {
		session.RequestSTTStop()
	}
	log.Printf("Twilio audio stream disconnected for session %s, total frames=%d, dropped=%d",
		sessionID, frameCount, droppedFrames)
}

// SendMediaToStream sends audio back to the Twilio stream (bidirectional)
// Audio must be base64-encoded mulaw 8kHz
func (h *CallHandler) SendMediaToStream(sessionID string, audioPayload string) error {
	session, err := h.sessionRepo.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.StreamSid == "" {
		return fmt.Errorf("stream not started for session: %s", sessionID)
	}

	// Note: Sending media requires storing the WebSocket connection in the session
	// For now, this is a placeholder for the bidirectional capability
	log.Printf("SendMediaToStream called for session %s (streamSid=%s)", sessionID, session.StreamSid)
	return nil
}

// ClearStreamBuffer clears any buffered audio on the Twilio stream
// Useful for interrupting TTS playback
func (h *CallHandler) ClearStreamBuffer(sessionID string) error {
	session, err := h.sessionRepo.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.StreamSid == "" {
		return fmt.Errorf("stream not started for session: %s", sessionID)
	}

	// Note: Clearing buffer requires stored WebSocket connection
	log.Printf("ClearStreamBuffer called for session %s (streamSid=%s)", sessionID, session.StreamSid)
	return nil
}

// ==========================================
// New Session-based API handlers
// ==========================================

// StartSession creates a new call session and initiates the PSTN call
// POST /api/sessions/start
// Input: {"to": "+91xxxx"}
// Output: {"sessionId": "...", "callSid": "...", "status": "dialing"}
func (h *CallHandler) StartSession(c *fiber.Ctx) error {
	type Request struct {
		To string `json:"to"`
	}

	var req Request
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.To == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing 'to' phone number"})
	}

	// Create session in repository
	session, err := h.sessionRepo.CreateSession(req.To)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create session"})
	}

	// Update session status to dialing
	_ = h.sessionRepo.SetStatus(session.ID, model.CallSessionStatusDialing)

	// Place call using Twilio with session-based TwiML URL
	ctx := context.Background()
	call, err := h.telephony.PlaceCallWithSession(ctx, session.ID, req.To)
	if err != nil {
		log.Printf("Failed to place call for session %s: %v", session.ID, err)
		_ = h.sessionRepo.SetStatus(session.ID, model.CallSessionStatusFailed)
		return c.Status(500).JSON(fiber.Map{
			"error":     "Failed to place call",
			"sessionId": session.ID,
		})
	}

	// Update session with call SID
	_ = h.sessionRepo.SetCallSid(session.ID, call.SID)

	log.Printf("Session started: sessionId=%s, callSid=%s, to=%s", session.ID, call.SID, req.To)

	return c.JSON(fiber.Map{
		"sessionId": session.ID,
		"callSid":   call.SID,
		"status":    "dialing",
	})
}

// HandleTwiML responds with TwiML for Twilio to start media streaming
// POST /twilio/twiml?sessionId=...
func (h *CallHandler) HandleTwiML(c *fiber.Ctx) error {
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		log.Println("TwiML webhook: missing sessionId")
		return c.Status(400).SendString("Missing sessionId")
	}

	// Verify session exists
	_, err := h.sessionRepo.GetSession(sessionID)
	if err != nil {
		log.Printf("TwiML webhook: session not found: %s", sessionID)
		return c.Status(404).SendString("Session not found")
	}

	// Get public WebSocket URL from config
	streamURL := fmt.Sprintf("%s/twilio/audio-stream", h.cfg.PublicWSURL)

	// Build TwiML response with Parameter element to pass sessionId
	// Twilio strips query params from WebSocket URLs, so we use <Parameter> instead
	// The sessionId will be sent in the WebSocket "start" message's customParameters
	twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
	<Connect>
		<Stream url="%s">
			<Parameter name="sessionId" value="%s"/>
		</Stream>
	</Connect>
</Response>`, streamURL, sessionID)

	log.Printf("TwiML response for session %s: Stream URL=%s", sessionID, streamURL)

	c.Set("Content-Type", "text/xml")
	return c.SendString(twiml)
}

// HandleIncomingTwilioCall handles completely new inbound PSTN calls from Twilio
// POST /twilio/incoming
func (h *CallHandler) HandleIncomingTwilioCall(c *fiber.Ctx) error {
	from := c.FormValue("From")
	callSid := c.FormValue("CallSid")

	if callSid == "" {
		log.Println("Incoming webhook: missing CallSid")
		return c.Status(400).SendString("Missing CallSid")
	}

	ctx := c.Context() // Use Fiber's context for request-scoped operations

	// Register an inbound bridge session
	session, err := h.bridge.RegisterInboundSession(ctx, callSid, from)
	if err != nil {
		log.Printf("Failed to register inbound session: %v", err)
		return c.Status(500).SendString("Internal Server Error")
	}

	// Link to model session for ML sync and app events
	modelSession := model.NewCallSession(session.ID, from)
	modelSession.CallSid = callSid
	modelSession.AudioEncoding = "mulaw"
	modelSession.SampleRate = 8000
	h.sessionRepo.AddSession(modelSession)

	// For legacy support, also add to sessionRepo if needed
	// h.sessionRepo.CreateSession(from) etc. - but RegisterInboundSession is better for the call loop

	// Log the incoming call
	log.Printf("Receiving incoming Twilio call: From=%s, CallSid=%s, SessionID=%s", from, callSid, session.ID)

	// Broadcast the incoming call event to all global listeners
	h.BroadcastGlobalEvent(AppEvent{
		Type:      "incoming_call",
		Status:    "ringing",
		Message:   from,
		Text:      session.ID,
		Timestamp: time.Now().UnixMilli(),
	})

	// Get public WebSocket URL from config
	streamURL := fmt.Sprintf("%s/twilio/audio-stream", h.cfg.PublicWSURL)

	// Return TwiML to wait for Answer and connect the stream
	twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
	<Say>Please wait while we connect your call to the Signify App.</Say>
	<Connect>
		<Stream url="%s">
			<Parameter name="sessionId" value="%s"/>
		</Stream>
	</Connect>
</Response>`, streamURL, session.ID)

	c.Set("Content-Type", "text/xml")
	return c.SendString(twiml)
}

// GetSessionRepo returns the session repository for external access
func (h *CallHandler) GetSessionRepo() *repository.CallSessionRepository {
	return h.sessionRepo
}

// GetTelephonyProvider returns the telephony provider for external access
func (h *CallHandler) GetTelephonyProvider() telephony.TelephonyProvider {
	return h.telephony
}

// ==========================================
// Global Events WebSocket (For Ringing/Incoming Calls)
// ==========================================

// HandleGlobalEventsWS handles connections that listen for global events like incoming calls
// GET /ws/app/global
func (h *CallHandler) HandleGlobalEventsWS(c *fiberwebsocket.Conn) {
	log.Println("Global events WS connected")

	h.GlobalMu.Lock()
	h.GlobalAppConns[c] = true
	h.GlobalMu.Unlock()

	defer func() {
		h.GlobalMu.Lock()
		delete(h.GlobalAppConns, c)
		h.GlobalMu.Unlock()
		c.Close()
		log.Println("Global events WS disconnected")
	}()

	for {
		if _, _, err := c.ReadMessage(); err != nil {
			break
		}
	}
}

// BroadcastGlobalEvent sends an event to all global listeners
func (h *CallHandler) BroadcastGlobalEvent(event AppEvent) {
	h.GlobalMu.Lock()
	defer h.GlobalMu.Unlock()

	for conn := range h.GlobalAppConns {
		if err := conn.WriteJSON(event); err != nil {
			log.Printf("Failed to broadcast to global conn: %v", err)
			conn.Close()
			delete(h.GlobalAppConns, conn)
		}
	}
}

// ==========================================
// App Events WebSocket (Milestone 2)
// ==========================================

// HandleAppEventsWS handles WebSocket connections for pushing events to the Over app
// GET /ws/app/events?sessionId=...
func (h *CallHandler) HandleAppEventsWS(c *fiberwebsocket.Conn) {
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		log.Println("App events WS: missing sessionId")
		c.Close()
		return
	}

	// Lookup session
	session, err := h.sessionRepo.GetSession(sessionID)
	if err != nil {
		if h.cfg.AllowTestSessions && strings.HasPrefix(sessionID, "test-session-") {
			log.Printf("App events WS: Auto-creating test session: %s", sessionID)
			session = model.NewCallSession(sessionID, "+0000000000")
			h.sessionRepo.AddSession(session)
		} else {
			log.Printf("App events WS: session not found: %s", sessionID)
			c.WriteJSON(AppEvent{Type: "error", Message: "Session not found"})
			c.Close()
			return
		}
	}

	// Set this connection as the app events sink (replaces old connection if exists)
	session.SetAppEventsConn(c)
	log.Printf("App events WS connected: sessionId=%s", sessionID)

	// Send current status
	session.SendAppEvent(AppEvent{
		Type:      "call_status",
		Status:    string(session.Status),
		Timestamp: time.Now().UnixMilli(),
	})

	// Keep connection alive and handle incoming messages
	for {
		_, _, err := c.ReadMessage()
		if err != nil {
			log.Printf("App events WS disconnected: sessionId=%s, err=%v", sessionID, err)
			break
		}
		// We don't expect messages from the app on this connection
		// This is primarily for pushing events TO the app
	}

	// Cleanup on disconnect
	session.SetAppEventsConn(nil)
}

// ==========================================
// STT Worker (Milestone 2)
// ==========================================

// startSTTWorker starts the STT processing for a session.
// It reads audio from session.AudioIn and pushes it to Deepgram.
// On Deepgram idle timeout (net0001), it retries automatically with backoff.
func (h *CallHandler) startSTTWorker(session *model.CallSession) {
	if !h.sttService.IsConfigured() {
		log.Printf("STT worker: Deepgram not configured, skipping for session %s", session.ID)
		session.MarkSTTWorkerStopped()
		return
	}

	defer session.MarkSTTWorkerStopped()

	const maxNet0001Retries = 3
	retryCount := 0

	log.Printf("STT worker starting for session %s", session.ID)

	for {
		if session.IsSTTStopRequested() {
			log.Printf("STT worker stopping before start (signal) for session %s", session.ID)
			return
		}

		deepgramErrCh := make(chan error, 1)

		// Create callbacks for STT results
		onPartial := func(text string, timestampMs int64) {
			session.SendAppEvent(AppEvent{
				Type:      "stt_partial",
				Text:      text,
				Timestamp: timestampMs,
			})
		}

		onFinal := func(text string, timestampMs int64) {
			session.SendAppEvent(AppEvent{
				Type:      "stt_final",
				Text:      text,
				Timestamp: timestampMs,
			})
			log.Printf("STT final for session %s: %s", session.ID, text)
		}

		onError := func(err error) {
			select {
			case deepgramErrCh <- err:
			default:
			}
		}

		// Start Deepgram session
		sttSession, err := h.sttService.StartSession(session.ID, session.AudioEncoding, session.SampleRate, session.STTLanguage, onPartial, onFinal, onError)
		if err != nil {
			log.Printf("Failed to start STT session for %s: %v", session.ID, err)
			session.SendAppEvent(AppEvent{
				Type:    "error",
				Message: fmt.Sprintf("STT error: %v", err),
			})
			return
		}

		log.Printf("STT worker running for session %s", session.ID)

		restartWorker := false
		for !restartWorker {
			select {
			case <-session.STTStopChan:
				log.Printf("STT worker stopping (signal) for session %s", session.ID)
				sttSession.Stop()
				sttSession.Wait()
				return

			case dgErr := <-deepgramErrCh:
				sttSession.Stop()
				sttSession.Wait()

				if isDeepgramNet0001Error(dgErr) && retryCount < maxNet0001Retries && !session.IsSTTStopRequested() {
					retryCount++
					backoff := time.Duration(1<<(retryCount-1)) * time.Second
					log.Printf("STT net0001 timeout for session %s; retry %d/%d in %s",
						session.ID, retryCount, maxNet0001Retries, backoff)
					time.Sleep(backoff)
					restartWorker = true
					break
				}

				log.Printf("STT error for session %s: %v", session.ID, dgErr)
				session.SendAppEvent(AppEvent{
					Type:    "error",
					Message: fmt.Sprintf("STT error: %v", dgErr),
				})
				return

			case audio, ok := <-session.AudioIn:
				if !ok {
					log.Printf("STT worker stopping (audio channel closed) for session %s", session.ID)
					sttSession.Stop()
					sttSession.Wait()
					return
				}

				// Push audio to Deepgram
				if err := sttSession.PushAudio(audio); err != nil {
					log.Printf("Failed to push audio to STT for session %s: %v", session.ID, err)
				}
			}
		}
	}
}

func (h *CallHandler) startSTTWorkerIfNeeded(session *model.CallSession, reason string) {
	if !h.sttService.IsConfigured() {
		return
	}

	if !session.TryStartSTTWorker() {
		return
	}

	log.Printf("Starting STT worker for session %s (trigger=%s)", session.ID, reason)
	go h.startSTTWorker(session)
}

func isDeepgramNet0001Error(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "net0001") || strings.Contains(errMsg, "did not receive audio data")
}

// SendCallStatusEvent sends a call status event to the app
func (h *CallHandler) SendCallStatusEvent(sessionID string, status model.CallSessionStatus) {
	session, err := h.sessionRepo.GetSession(sessionID)
	if err != nil {
		return
	}

	session.SendAppEvent(AppEvent{
		Type:      "call_status",
		Status:    string(status),
		Timestamp: time.Now().UnixMilli(),
	})
}

// ==========================================
// Landmarks WebSocket (Milestone 3)
// ==========================================

// HandleLandmarksWS handles WebSocket connections for receiving landmarks from the Over app
// GET /ws/app/landmarks?sessionId=...
func (h *CallHandler) HandleLandmarksWS(c *fiberwebsocket.Conn) {
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		log.Println("Landmarks WS: missing sessionId")
		c.WriteJSON(map[string]string{"type": "error", "message": "Missing sessionId"})
		c.Close()
		return
	}

	// Get session
	session, err := h.sessionRepo.GetSession(sessionID)
	if err != nil {
		if h.cfg.AllowTestSessions && strings.HasPrefix(sessionID, "test-session-") {
			log.Printf("Landmarks WS: Auto-creating test session: %s", sessionID)
			session = model.NewCallSession(sessionID, "+0000000000")
			h.sessionRepo.AddSession(session)
		} else {
			log.Printf("Landmarks WS: session not found: %s", sessionID)
			c.WriteJSON(map[string]string{"type": "error", "message": "Session not found"})
			c.Close()
			return
		}
	}

	// Set connection
	session.SetLandmarksConn(c)
	log.Printf("Landmarks WS connected: sessionId=%s", sessionID)

	// Register Sentence Builder Callback (Milestone 5)
	h.sentenceBuilder.RegisterCallback(sessionID, func(text string, isFinal bool) {
		// Clean up the text (remove ASL: and ISL: prefixes)
		cleanText := strings.TrimPrefix(text, "ASL: ")
		cleanText = strings.TrimPrefix(cleanText, "ISL: ")
		// Also handle middle ones if repeated "ASL: word ASL: word"
		cleanText = strings.ReplaceAll(cleanText, " ASL: ", " ")
		cleanText = strings.ReplaceAll(cleanText, " ISL: ", " ")

		eventType := "sign_partial"
		if isFinal {
			eventType = "sign_final"
		}

		event := AppEvent{
			Type:      eventType,
			Text:      cleanText,
			Timestamp: time.Now().UnixMilli(),
		}

		// If it's a test session and final, generate TTS so the user can hear it for verification
		if isFinal && strings.HasPrefix(sessionID, "test-session-") {
			log.Printf("Test Mode: Generating TTS for verification: %s", cleanText)
			// Use landmark processor's GenerateTTS
			audio, err := h.landmarkProcessor.GenerateTTS(context.Background(), cleanText)
			if err != nil {
				log.Printf("Test Mode TTS error: %v", err)
			} else if audio != nil {
				log.Printf("Test Mode: Generated audio length: %d bytes", len(audio))
				event.Audio = base64.StdEncoding.EncodeToString(audio)
			}
		}

		session.SendAppEvent(event)
	})

	// Set Language Preference (Milestone 6)
	lang := c.Query("language")
	if lang != "" {
		session.Language = lang
		log.Printf("Session %s language set to: %s", sessionID, lang)
	} else {
		session.Language = "ASL" // Default
	}

	// Start landmark worker if not already running
	h.startLandmarkWorker(session)

	// Stats for logging
	var frameCount int
	var dropCount int
	var lastLogTime = time.Now()

	// Rate limiting
	var lastFrameTime = time.Now()
	const minFrameInterval = 40 * time.Millisecond // Max ~25 FPS

	// Read landmark packets
	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			log.Printf("Landmarks WS disconnected: sessionId=%s, err=%v", sessionID, err)
			break
		}

		// Parse packet
		var pkt model.LandmarkPacket
		if err := json.Unmarshal(msg, &pkt); err != nil {
			log.Printf("Landmarks WS: invalid JSON: %v", err)
			continue
		}

		// Validate sessionId
		if pkt.SessionID != "" && pkt.SessionID != sessionID {
			log.Printf("Landmarks WS: sessionId mismatch: got %s, expected %s", pkt.SessionID, sessionID)
			continue
		}

		// Rate limit check
		if time.Since(lastFrameTime) < minFrameInterval {
			dropCount++
			continue
		}
		lastFrameTime = time.Now()

		// Validate present/landmarks consistency
		if pkt.Present {
			// Updated for full holistic: Pose(33×3) + Face(468×3) + LeftHand(21×3) + RightHand(21×3) = 1629 floats
			if len(pkt.Landmarks) != 1629 {
				log.Printf("Landmarks WS: invalid landmarks length: got %d, expected 1629", len(pkt.Landmarks))
				continue
			}
			// Check for NaN/Inf
			valid := true
			for _, v := range pkt.Landmarks {
				if v != v || (v > 1e10) || (v < -1e10) { // NaN check + reasonable bounds
					valid = false
					break
				}
			}
			if !valid {
				log.Printf("Landmarks WS: invalid landmark values (NaN or out of bounds)")
				continue
			}
		} else {
			if len(pkt.Landmarks) != 0 {
				pkt.Landmarks = nil // Normalize: present=false means empty landmarks
			}
		}

		// Push to session window
		session.PushLandmark(pkt)
		frameCount++

		// Log stats every 5 seconds
		if time.Since(lastLogTime) > 5*time.Second {
			window := session.GetLandmarkWindow()
			fps := float64(frameCount) / time.Since(lastLogTime).Seconds()
			log.Printf("Landmarks WS stats: sessionId=%s, fps=%.1f, windowSize=%d, dropped=%d",
				sessionID, fps, len(window), dropCount)
			frameCount = 0
			dropCount = 0
			lastLogTime = time.Now()
		}
	}

	// Cleanup
	h.stopLandmarkWorker(session)
	session.ClearLandmarkWindow()
	h.stabilizer.ClearSession(sessionID)
	h.sentenceBuilder.ClearSession(sessionID)
	session.SetLandmarksConn(nil)
	log.Printf("Landmarks WS cleanup: sessionId=%s", sessionID)
}

func (h *CallHandler) stopLandmarkWorker(session *model.CallSession) {
	session.LandmarkMu.Lock()
	defer session.LandmarkMu.Unlock()

	if !session.LandmarkActive {
		return
	}

	close(session.LandmarkStop)
	session.LandmarkStop = make(chan struct{})
	session.LandmarkActive = false
}

// startLandmarkWorker starts the landmark processing worker for the session
func (h *CallHandler) startLandmarkWorker(session *model.CallSession) {
	if h.inferenceClient == nil {
		log.Printf("Landmark worker disabled for session %s: inference client unavailable", session.ID)
		return
	}

	session.LandmarkMu.Lock()
	if session.LandmarkActive {
		session.LandmarkMu.Unlock()
		return
	}
	session.LandmarkActive = true
	session.LandmarkMu.Unlock()

	log.Printf("Landmark worker starting for session %s", session.ID)

	go func() {
		ticker := time.NewTicker(200 * time.Millisecond) // Check every 200ms for responsiveness
		defer ticker.Stop()

		var lastEmitTime time.Time

		for {
			select {
			case <-session.LandmarkStop:
				log.Printf("Landmark worker stopped for session %s", session.ID)
				return

			case <-ticker.C:
				window := session.GetLandmarkWindow()

				// Only emit when window has enough data
				if len(window) >= 15 { // Reduced from 30 for faster feedback during testing
					// Rate limit inference
					if time.Since(lastEmitTime) >= 500*time.Millisecond {
						// Find the latest valid frame
						var latestFrame *model.LandmarkPacket
						for i := len(window) - 1; i >= 0; i-- {
							if len(window[i].Landmarks) == 1629 {
								latestFrame = &window[i]
								break
							}
						}

						if latestFrame == nil {
							// Expected if hands aren't present or frames dropped
							continue
						}

						// Convert window to [][]float32 for inference, skipping empty frames
						var windowF32 [][]float32
						for _, frame := range window {
							if len(frame.Landmarks) != 1629 {
								continue
							}
							f32 := make([]float32, 1629)
							for j, val := range frame.Landmarks {
								f32[j] = float32(val)
							}
							windowF32 = append(windowF32, f32)
						}

						if len(windowF32) < 5 {
							// Not enough valid frames for reliable inference
							continue
						}

						// ACTIVITY DETECTION (Milestone 4.4b)
						// Check if hands are "active" (raised above a certain threshold)
						// Normalized coordinates: y=0 (top), y=1 (bottom)
						// Threshold: 0.7 (approx chest/stomach level). If > 0.7, hand is "dropped".
						activeThreshold := 0.7

						// Offsets (from mediapipe_holistic_service.dart)
						// Left Hand Starts: 33*3 + 468*3 = 99 + 1404 = 1503
						// Right Hand Starts: 1503 + 21*3 = 1503 + 63 = 1566
						lhWristY := latestFrame.Landmarks[1503+1] // +1 for Y
						rhWristY := latestFrame.Landmarks[1566+1] // +1 for Y

						// Check Left Hand (if not all zeros)
						lhActive := false
						if latestFrame.Landmarks[1503] != 0 || latestFrame.Landmarks[1503+1] != 0 {
							if lhWristY < float64(activeThreshold) {
								lhActive = true
							}
						}

						// Check Right Hand (if not all zeros)
						rhActive := false
						if latestFrame.Landmarks[1566] != 0 || latestFrame.Landmarks[1566+1] != 0 {
							if rhWristY < float64(activeThreshold) {
								rhActive = true
							}
						}

						// If NO hands are active, skip inference
						// (Using OR logic: if at least one hand is up, we predict)
						if !lhActive && !rhActive {
							// log.Println("Skipping inference: Hands dropped/idle")
							continue
						}

						// Call Inference Service
						// Append language to sessionID for routing (hack to avoid proto change)
						inferenceSessionID := session.ID
						if session.Language != "" {
							inferenceSessionID = session.ID + "|" + session.Language
						}
						predictions, err := h.inferenceClient.Predict(inferenceSessionID, windowF32)
						if err != nil {
							continue
						}

						if len(predictions) > 0 {
							top := predictions[0]
							// Confidence threshold (Increased to 0.8)
							if top.Confidence > 0.8 {
								// Run through stabilizer
								stableSign := h.stabilizer.AddPrediction(session.ID, top.Label, top.Confidence)

								if stableSign != "" {
									// Milestone 5: Feed to Sentence Builder
									h.sentenceBuilder.AddWord(session.ID, stableSign)

									lastEmitTime = time.Now()
									log.Printf("STABILIZED SIGN: %s for session %s", stableSign, session.ID)
								}
							}
						}
					}
				}
			}
		}
	}()
}

// Exotel webhook handlers

// HandleExotelIncoming handles new inbound calls from Exotel (India)
func (h *CallHandler) HandleExotelIncoming(c *fiber.Ctx) error {
	from := c.FormValue("From")
	callSid := c.FormValue("CallSid")

	if callSid == "" {
		return c.Status(400).SendString("Missing CallSid")
	}

	ctx := c.Context()
	session, err := h.bridge.RegisterInboundSession(ctx, callSid, from)
	if err != nil {
		return c.Status(500).SendString("Internal Error")
	}

	// Link to model session for ML sync and app events
	modelSession := model.NewCallSession(session.ID, from)
	modelSession.CallSid = callSid
	modelSession.AudioEncoding = "linear16"
	modelSession.SampleRate = 8000
	h.sessionRepo.AddSession(modelSession)

	log.Printf("Incoming Exotel call: From=%s, CallSid=%s, SessionID=%s", from, callSid, session.ID)

	h.BroadcastGlobalEvent(AppEvent{
		Type:      "incoming_call",
		Status:    "ringing",
		Message:   from,
		Text:      session.ID,
		Timestamp: time.Now().UnixMilli(),
	})

	// For Exotel, we'll redirect to the Voicebot Applet logic
	// This usually happens by returning specific passthrough parameters if using a Flow,
	// or returning a Voicebot JSON response if configured as a Voicebot URL.
	// We'll return TwiML-like XML if the user uses Exotel's Passthru.

	streamURL := fmt.Sprintf("%s/exotel/audio-stream", h.cfg.PublicWSURL)

	// Exotel XML response for Voicebot/Stream integration
	response := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Say>Connecting to Signify.</Say>
    <Connect>
        <Stream url="%s">
            <Parameter name="sessionId" value="%s"/>
        </Stream>
    </Connect>
</Response>`, streamURL, session.ID)

	c.Set("Content-Type", "text/xml")
	return c.SendString(response)
}

// HandleExotelAudioStream handles Exotel bidirectional WebSocket streams
func (h *CallHandler) HandleExotelAudioStream(c *fiberwebsocket.Conn) {
	log.Println("Exotel audio stream: WebSocket connected")

	var sessionID string
	var session *model.CallSession
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handler for sending audio back to Exotel
	go func() {
		var bridgeSession *telephony.BridgeSession
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if sessionID == "" {
					time.Sleep(100 * time.Millisecond)
					continue
				}

				if bridgeSession == nil {
					b, exists := h.bridge.GetSession(sessionID)
					if exists {
						bridgeSession = b
					} else {
						time.Sleep(100 * time.Millisecond)
						continue
					}
				}

				// Wait for audio from TTS (Bridge handles the Sign-to-Voice pipeline)
				select {
				case <-ctx.Done():
					return
				case audio, ok := <-bridgeSession.TTSAudioOut:
					if !ok || audio == nil {
						if !ok {
							return
						}
						continue
					}
					// Wrap in Exotel JSON format
					payload := base64.StdEncoding.EncodeToString(audio)
					msg := map[string]interface{}{
						"event": "media",
						"media": map[string]string{
							"payload": payload,
						},
					}
					jsonMsg, _ := json.Marshal(msg)
					if err := c.WriteMessage(fiberwebsocket.TextMessage, jsonMsg); err != nil {
						log.Printf("Failed to write to Exotel stream: %v", err)
						return
					}
				}
			}
		}
	}()

	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			break
		}

		var streamMsg struct {
			Event string `json:"event"`
			Start struct {
				CustomParameters map[string]string `json:"customParameters"`
			} `json:"start"`
			Media struct {
				Payload string `json:"payload"`
			} `json:"media"`
		}

		if err := json.Unmarshal(msg, &streamMsg); err != nil {
			continue
		}

		switch streamMsg.Event {
		case "start":
			sessionID = streamMsg.Start.CustomParameters["sessionId"]
			if sessionID == "" {
				c.Close()
				return
			}
			session, err = h.sessionRepo.GetSession(sessionID)
			if err != nil {
				if h.cfg.AllowTestSessions && strings.HasPrefix(sessionID, "test-session-") {
					log.Printf("Exotel stream: Auto-creating test session: %s", sessionID)
					session = model.NewCallSession(sessionID, "+0000000000")
					h.sessionRepo.AddSession(session)
				} else {
					log.Printf("Exotel stream: session not found: %s", sessionID)
					c.Close()
					return
				}
			}
			log.Printf("Exotel stream started for session: %s", sessionID)

		case "media":
			if session == nil || session.AudioIn == nil {
				continue
			}
			audioData, _ := base64.StdEncoding.DecodeString(streamMsg.Media.Payload)
			session.PushAudio(audioData)
			h.startSTTWorkerIfNeeded(session, "exotel_media")
		}
	}
}

// ==========================================
// App Audio WebSocket (Test Mode STT)
// ==========================================

// HandleAppAudioWS handles raw PCM16 @ 16kHz audio from Flutter mic for Deepgram STT testing.
// Flutter connects here when in test mode to simulate the hearing caller's voice.
// GET /ws/app/audio?sessionId=...
// Binary frames: raw PCM16 little-endian, 16 kHz, mono
func (h *CallHandler) HandleAppAudioWS(c *fiberwebsocket.Conn) {
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		log.Println("App audio WS: missing sessionId")
		c.WriteJSON(map[string]string{"type": "error", "message": "Missing sessionId"})
		c.Close()
		return
	}

	// Lookup or auto-create session
	session, err := h.sessionRepo.GetSession(sessionID)
	if err != nil {
		if h.cfg.AllowTestSessions && strings.HasPrefix(sessionID, "test-session-") {
			log.Printf("App audio WS: Auto-creating test session: %s", sessionID)
			session = model.NewCallSession(sessionID, "+0000000000")
			h.sessionRepo.AddSession(session)
		} else {
			log.Printf("App audio WS: session not found: %s", sessionID)
			c.WriteJSON(map[string]string{"type": "error", "message": "Session not found"})
			c.Close()
			return
		}
	}

	// Configure session for Flutter mic audio: linear16 @ 16kHz
	session.AudioEncoding = "linear16"
	session.SampleRate = 16000

	// Set STT language from query param (BCP-47, e.g. "en-US", "hi").
	// Defaults to "en-US" if not provided.
	if lang := c.Query("language"); lang != "" {
		session.STTLanguage = lang
	} else if session.STTLanguage == "" {
		session.STTLanguage = "en-US"
	}

	// Reset STT state so the worker can be (re)started even if a previous
	// audio WS connection already closed and called RequestSTTStop().
	session.ResetSTTState()

	log.Printf("App audio WS connected: sessionId=%s (linear16@16kHz, lang=%s)", sessionID, session.STTLanguage)

	var frameCount int

	for {
		msgType, data, err := c.ReadMessage()
		if err != nil {
			log.Printf("App audio WS disconnected: sessionId=%s, frames=%d, err=%v",
				sessionID, frameCount, err)
			break
		}

		// Accept binary frames only
		if msgType != fiberwebsocket.BinaryMessage {
			continue
		}

		if len(data) == 0 {
			continue
		}

		// Push raw PCM16 frames to session audio pipeline
		if session.PushAudio(data) {
			// Start STT worker on first frame
			h.startSTTWorkerIfNeeded(session, "app_mic")
			frameCount++
			if frameCount%200 == 0 {
				log.Printf("App audio WS: session=%s, frames=%d", sessionID, frameCount)
			}
		}
	}

	// Signal STT to flush and stop
	session.RequestSTTStop()
}
