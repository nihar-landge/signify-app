package api

import (
	"signify/internal/repository"
	"signify/internal/service"
	"signify/internal/websocket"

	fiberwebsocket "github.com/gofiber/websocket/v2"
)

type Handler struct {
	streamProcessor *service.StreamProcessor
	hub             *websocket.Hub
	sessionManager  *service.SessionManager
	postgresRepo    *repository.PostgresRepository
}

func NewHandler(sp *service.StreamProcessor, hub *websocket.Hub, sm *service.SessionManager, pg *repository.PostgresRepository) *Handler {
	return &Handler{
		streamProcessor: sp,
		hub:             hub,
		sessionManager:  sm,
		postgresRepo:    pg,
	}
}

func (h *Handler) HandleWebSocket(c *fiberwebsocket.Conn) {
	userID := c.Params("userID")

	// Create new client
	client := &websocket.Client{
		Hub:             h.hub,
		Conn:            c,
		Send:            make(chan []byte, 256),
		UserID:          userID,
		StreamProcessor: h.streamProcessor,
	}

	// Register with Hub
	client.Hub.Register <- client

	// Start pump (Blocking call, Fiber handles concurrency)
	// Note: ReadPump blocks until connection close
	client.ReadPump()

	// WritePump would typically be a separate goroutine,
	// but ReadPump logic above handles basic IO for this stub.
	// In a full implementation, you'd split them properly to handle broadcast.
}
