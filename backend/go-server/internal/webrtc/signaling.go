package webrtc

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gofiber/websocket/v2"
)

// SignalingMessage represents a WebRTC signaling message
type SignalingMessage struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// PeerSession represents a WebRTC peer connection session
type PeerSession struct {
	ID          string
	UserID      string
	Conn        *websocket.Conn
	PhoneNumber string
	BridgeID    string // Link to telephony bridge session

	// Callbacks
	OnLandmarks     func(data []byte)
	OnCallInitiated func(phoneNumber string) (bridgeID string, err error)
	OnCallEnded     func()
}

// SignalingHub manages WebRTC signaling connections
type SignalingHub struct {
	sessions map[string]*PeerSession // userID -> session
	mu       sync.RWMutex

	// External handlers
	onCallInitiate func(userID, phoneNumber string) (bridgeID string, err error)
	onCallAccept   func(userID, sessionID string) error
	onCallEnd      func(userID string)
	onLandmarks    func(userID string, data []byte)
}

// SetCallAcceptHandler sets the handler for call acceptance
func (h *SignalingHub) SetCallAcceptHandler(handler func(userID, sessionID string) error) {
	h.onCallAccept = handler
}

// NewSignalingHub creates a new signaling hub
func NewSignalingHub() *SignalingHub {
	return &SignalingHub{
		sessions: make(map[string]*PeerSession),
	}
}

// SetCallInitiateHandler sets the handler for call initiation
func (h *SignalingHub) SetCallInitiateHandler(handler func(userID, phoneNumber string) (string, error)) {
	h.onCallInitiate = handler
}

// SetCallEndHandler sets the handler for call end
func (h *SignalingHub) SetCallEndHandler(handler func(userID string)) {
	h.onCallEnd = handler
}

// SetLandmarksHandler sets the handler for landmark data
func (h *SignalingHub) SetLandmarksHandler(handler func(userID string, data []byte)) {
	h.onLandmarks = handler
}

// HandleConnection handles a new WebSocket signaling connection
func (h *SignalingHub) HandleConnection(conn *websocket.Conn, userID string) {
	session := &PeerSession{
		ID:     userID,
		UserID: userID,
		Conn:   conn,
	}

	h.mu.Lock()
	h.sessions[userID] = session
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.sessions, userID)
		h.mu.Unlock()

		if h.onCallEnd != nil {
			h.onCallEnd(userID)
		}

		conn.Close()
	}()

	log.Printf("Signaling connection established for user %s", userID)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Signaling read error for user %s: %v", userID, err)
			return
		}

		var sigMsg SignalingMessage
		if err := json.Unmarshal(msg, &sigMsg); err != nil {
			log.Printf("Invalid signaling message from user %s: %v", userID, err)
			continue
		}

		h.handleMessage(session, sigMsg)
	}
}

// handleMessage processes an incoming signaling message
func (h *SignalingHub) handleMessage(session *PeerSession, msg SignalingMessage) {
	switch msg.Type {
	case "call_initiate":
		h.handleCallInitiate(session, msg.Data)
	case "call_accept":
		h.handleCallAccept(session, msg.Data)
	case "call_end":
		h.handleCallEnd(session)
	case "ice_candidate":
		h.handleICECandidate(session, msg.Data)
	case "landmarks":
		h.handleLandmarks(session, msg.Data)
	default:
		log.Printf("Unknown signaling message type: %s", msg.Type)
	}
}

// handleCallAccept processes a request to answer an incoming call
func (h *SignalingHub) handleCallAccept(session *PeerSession, data map[string]interface{}) {
	sessionID, _ := data["session_id"].(string)
	if sessionID == "" {
		h.sendError(session, "Missing session_id for call_accept")
		return
	}

	log.Printf("Call accept from user %s for session %s", session.UserID, sessionID)

	// Link this sessionID to the Bridge session
	if h.onCallAccept != nil {
		if err := h.onCallAccept(session.UserID, sessionID); err != nil {
			h.sendError(session, "Failed to accept call: "+err.Error())
			return
		}
		session.BridgeID = sessionID
	}

	// Notify that call is connected
	h.sendMessage(session, SignalingMessage{
		Type: "call_connected",
		Data: map[string]interface{}{},
	})
}

// handleCallInitiate processes a call initiation request
func (h *SignalingHub) handleCallInitiate(session *PeerSession, data map[string]interface{}) {
	phoneNumber, ok := data["phone_number"].(string)
	if !ok || phoneNumber == "" {
		h.sendError(session, "Invalid phone number")
		return
	}

	session.PhoneNumber = phoneNumber

	// Get SDP offer
	offer, _ := data["offer"].(map[string]interface{})
	sdp, _ := offer["sdp"].(string)
	sdpType, _ := offer["type"].(string)

	log.Printf("Call initiate from user %s to %s", session.UserID, phoneNumber)

	// Initiate the call via the bridge
	if h.onCallInitiate != nil {
		bridgeID, err := h.onCallInitiate(session.UserID, phoneNumber)
		if err != nil {
			h.sendError(session, "Failed to initiate call: "+err.Error())
			return
		}
		session.BridgeID = bridgeID
	}

	// Create SDP answer
	// In a full implementation, this would use pion/webrtc
	// For now, we'll echo back a simplified answer
	answer := map[string]interface{}{
		"sdp":  sdp, // Echo for now
		"type": "answer",
	}

	if sdpType != "" {
		answer["type"] = "answer"
	}

	h.sendMessage(session, SignalingMessage{
		Type: "answer",
		Data: answer,
	})

	// Notify that call is ringing
	h.sendMessage(session, SignalingMessage{
		Type: "call_ringing",
		Data: map[string]interface{}{},
	})
}

// handleCallEnd processes a call end request
func (h *SignalingHub) handleCallEnd(session *PeerSession) {
	log.Printf("Call end from user %s", session.UserID)

	if h.onCallEnd != nil {
		h.onCallEnd(session.UserID)
	}

	h.sendMessage(session, SignalingMessage{
		Type: "call_ended",
		Data: map[string]interface{}{},
	})
}

// handleICECandidate processes an ICE candidate
func (h *SignalingHub) handleICECandidate(session *PeerSession, data map[string]interface{}) {
	// In a full implementation, this would add the ICE candidate to the peer connection
	// For now, we just echo it back
	log.Printf("ICE candidate from user %s", session.UserID)

	// Echo ICE candidate back (simplified)
	h.sendMessage(session, SignalingMessage{
		Type: "ice_candidate",
		Data: data,
	})
}

// handleLandmarks processes landmark data from the client
func (h *SignalingHub) handleLandmarks(session *PeerSession, data map[string]interface{}) {
	// Re-serialize the landmark data
	landmarkJSON, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to serialize landmarks: %v", err)
		return
	}

	if h.onLandmarks != nil {
		h.onLandmarks(session.UserID, landmarkJSON)
	}
}

// SendCaption sends a caption to a user
func (h *SignalingHub) SendCaption(userID, caption string) {
	h.mu.RLock()
	session, exists := h.sessions[userID]
	h.mu.RUnlock()

	if !exists {
		return
	}

	h.sendMessage(session, SignalingMessage{
		Type: "caption",
		Data: map[string]interface{}{
			"text": caption,
		},
	})
}

// SendCallConnected notifies that the call is connected
func (h *SignalingHub) SendCallConnected(userID string) {
	h.mu.RLock()
	session, exists := h.sessions[userID]
	h.mu.RUnlock()

	if !exists {
		return
	}

	h.sendMessage(session, SignalingMessage{
		Type: "call_connected",
		Data: map[string]interface{}{},
	})
}

// SendCallEnded notifies that the call has ended
func (h *SignalingHub) SendCallEnded(userID string) {
	h.mu.RLock()
	session, exists := h.sessions[userID]
	h.mu.RUnlock()

	if !exists {
		return
	}

	h.sendMessage(session, SignalingMessage{
		Type: "call_ended",
		Data: map[string]interface{}{},
	})
}

// sendMessage sends a signaling message to a session
func (h *SignalingHub) sendMessage(session *PeerSession, msg SignalingMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal signaling message: %v", err)
		return
	}

	if err := session.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("Failed to send signaling message to user %s: %v", session.UserID, err)
	}
}

// sendError sends an error message to a session
func (h *SignalingHub) sendError(session *PeerSession, errMsg string) {
	h.sendMessage(session, SignalingMessage{
		Type: "call_failed",
		Data: map[string]interface{}{
			"reason": errMsg,
		},
	})
}

// GetSession returns a session by user ID
func (h *SignalingHub) GetSession(userID string) (*PeerSession, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	session, exists := h.sessions[userID]
	return session, exists
}
