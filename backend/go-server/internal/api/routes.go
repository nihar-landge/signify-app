package api

import (
	"signify/internal/service"
	"signify/internal/websocket"

	"github.com/gofiber/fiber/v2"
	fiberwebsocket "github.com/gofiber/websocket/v2"
)

// SetupRoutes configures all application routes
func SetupRoutes(app *fiber.App, handler *Handler, callHandler *CallHandler) {
	// Apply global middleware
	app.Use(RecoveryMiddleware())
	app.Use(LoggingMiddleware())
	app.Use(CORSMiddleware())

	// Health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "signify-backend",
		})
	})

	// API v1 routes
	api := app.Group("/api/v1")

	// =====================================================
	// Landmarks WebSocket (Milestone 3 - Sign Language)
	// =====================================================

	// Landmarks WebSocket - receives hand landmarks from Over app
	// GET /ws/app/landmarks?sessionId=...
	app.Get("/ws/app/landmarks", func(c *fiber.Ctx) error {
		if fiberwebsocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return fiberwebsocket.New(callHandler.HandleLandmarksWS)(c)
		}
		return fiber.ErrUpgradeRequired
	})

	// Session endpoints
	api.Get("/sessions/:userID", handler.GetSessions)
	api.Delete("/sessions/:userID", handler.DeleteSession)

	// Conversation history endpoints
	api.Get("/conversations/:userID", handler.GetConversations)
	api.Get("/conversations/:userID/:conversationID", handler.GetConversationMessages)

	// =====================================================
	// Relay Call endpoints (legacy)
	// =====================================================

	// Call initiation and management (legacy - userID based)
	api.Post("/call/:userID/initiate", callHandler.InitiateCall)
	api.Post("/call/:userID/end", callHandler.EndCall)
	api.Get("/call/stats", func(c *fiber.Ctx) error {
		return c.JSON(callHandler.GetStats())
	})

	// =====================================================
	// New Session-based API (Milestone 1)
	// =====================================================

	// POST /api/sessions/start - Create session and start PSTN call
	app.Post("/api/sessions/start", callHandler.StartSession)

	// =====================================================
	// Twilio Webhook endpoints
	// =====================================================

	// Twilio signature middleware for webhook security
	// Validates X-Twilio-Signature header using HMAC-SHA1
	twilioMiddleware := TwilioSignatureMiddlewareFromEnv()

	// TwiML webhook - Twilio fetches this to get Stream instructions
	// POST /twilio/twiml?sessionId=...
	app.Post("/twilio/twiml", twilioMiddleware, callHandler.HandleTwiML)

	// Incoming call webhook - Twilio hits this when a real phone dials the Twilio number
	// POST /twilio/incoming
	app.Post("/twilio/incoming", twilioMiddleware, callHandler.HandleIncomingTwilioCall)

	// Status callback - receives call status updates
	// POST /twilio/status?sessionId=...
	app.Post("/twilio/status", twilioMiddleware, callHandler.HandleTwilioStatus)

	// Audio stream WebSocket - receives real-time audio from Twilio
	app.Use("/twilio/audio-stream", func(c *fiber.Ctx) error {
		if fiberwebsocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/twilio/audio-stream", fiberwebsocket.New(callHandler.HandleTwilioAudioStream))

	// =====================================================
	// Exotel Webhook endpoints (India)
	// =====================================================

	// Incoming call webhook - Exotel hits this when an Indian phone dials the Exotel number
	app.Post("/exotel/incoming", callHandler.HandleExotelIncoming)

	// Audio stream WebSocket - receives real-time audio from Exotel
	app.Use("/exotel/audio-stream", func(c *fiber.Ctx) error {
		if fiberwebsocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/exotel/audio-stream", fiberwebsocket.New(callHandler.HandleExotelAudioStream))

	// =====================================================
	// WebSocket endpoints
	// =====================================================

	// Original WebSocket for sign interpretation
	app.Use("/ws", func(c *fiber.Ctx) error {
		if fiberwebsocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws/:userID", fiberwebsocket.New(handler.HandleWebSocket))

	// Call signaling WebSocket
	app.Use("/call", func(c *fiber.Ctx) error {
		if fiberwebsocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/call/:userID", fiberwebsocket.New(callHandler.HandleCallWebSocket))

	// =====================================================
	// App Events WebSocket (Milestone 2 - STT captions)
	// =====================================================

	// Global events WebSocket - listens for incoming calls across all apps
	// GET /ws/app/global
	app.Use("/ws/app/global", func(c *fiber.Ctx) error {
		if fiberwebsocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws/app/global", fiberwebsocket.New(callHandler.HandleGlobalEventsWS))

	// App events WebSocket - pushes STT captions and call status to Over app
	// GET /ws/app/events?sessionId=...
	app.Use("/ws/app/events", func(c *fiber.Ctx) error {
		if fiberwebsocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws/app/events", fiberwebsocket.New(callHandler.HandleAppEventsWS))

	// App audio WebSocket - Flutter mic PCM16 @ 16kHz → Deepgram STT (test mode)
	// GET /ws/app/audio?sessionId=...
	app.Use("/ws/app/audio", func(c *fiber.Ctx) error {
		if fiberwebsocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws/app/audio", fiberwebsocket.New(callHandler.HandleAppAudioWS))
}

// Handler REST API methods

// GetSessions returns active sessions for a user
func (h *Handler) GetSessions(c *fiber.Ctx) error {
	userID := c.Params("userID")

	if h.sessionManager != nil {
		session, exists := h.sessionManager.GetSession(userID)
		if exists {
			return c.JSON(fiber.Map{
				"userID": userID,
				"session": fiber.Map{
					"conversationID": session.ConversationID,
					"startTime":      session.StartTime,
					"lastActivity":   session.LastActivity,
					"messageCount":   len(session.Messages),
				},
			})
		}
	}

	return c.JSON(fiber.Map{
		"userID":   userID,
		"sessions": []string{},
	})
}

// DeleteSession removes a user's session
func (h *Handler) DeleteSession(c *fiber.Ctx) error {
	userID := c.Params("userID")

	if h.sessionManager != nil {
		h.sessionManager.DeleteSession(userID)
	}

	return c.JSON(fiber.Map{
		"userID":  userID,
		"deleted": true,
	})
}

// GetConversations returns conversation history for a user
func (h *Handler) GetConversations(c *fiber.Ctx) error {
	userID := c.Params("userID")

	// Try PostgreSQL first for persisted history
	if h.postgresRepo != nil {
		conversations, err := h.postgresRepo.GetConversations(userID)
		if err == nil {
			return c.JSON(fiber.Map{
				"userID":        userID,
				"conversations": conversations,
			})
		}
		// Fall through to in-memory if DB query fails
	}

	// Fallback: return current session messages
	if h.sessionManager != nil {
		session, exists := h.sessionManager.GetSession(userID)
		if exists {
			return c.JSON(fiber.Map{
				"userID": userID,
				"conversations": []fiber.Map{
					{
						"id":         session.ConversationID,
						"title":      "Current Session",
						"created_at": session.StartTime,
						"updated_at": session.LastActivity,
					},
				},
			})
		}
	}

	return c.JSON(fiber.Map{
		"userID":        userID,
		"conversations": []interface{}{},
	})
}

// GetConversationMessages returns messages for a specific conversation
func (h *Handler) GetConversationMessages(c *fiber.Ctx) error {
	userID := c.Params("userID")
	conversationID := c.Params("conversationID")

	// Try PostgreSQL first
	if h.postgresRepo != nil {
		messages, err := h.postgresRepo.GetMessages(conversationID)
		if err == nil {
			return c.JSON(fiber.Map{
				"userID":         userID,
				"conversationID": conversationID,
				"messages":       messages,
			})
		}
	}

	// Fallback: return in-memory session messages
	if h.sessionManager != nil {
		messages := h.sessionManager.GetMessages(userID)
		return c.JSON(fiber.Map{
			"userID":         userID,
			"conversationID": conversationID,
			"messages":       messages,
		})
	}

	return c.JSON(fiber.Map{
		"userID":         userID,
		"conversationID": conversationID,
		"messages":       []interface{}{},
	})
}

// NewRouterHandler creates a handler with all dependencies
func NewRouterHandler(sp *service.StreamProcessor, hub *websocket.Hub) *Handler {
	return NewHandler(sp, hub, nil, nil)
}
