package websocket

import (
	"signify/internal/service"

	"github.com/gofiber/websocket/v2"
)

type Client struct {
	Hub             *Hub
	Conn            *websocket.Conn
	Send            chan []byte
	UserID          string
	StreamProcessor *service.StreamProcessor
}

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	// Channels for this session's processing
	videoFrameChan := make(chan []byte, 10)
	audioChunkChan := make(chan []byte, 10)
	textOutputChan := make(chan string, 10)
	audioOutputChan := make(chan []byte, 10)

	// Start processor for this client
	go c.StreamProcessor.ProcessVideoStream(c.UserID, videoFrameChan, textOutputChan, audioOutputChan)

	// Handle outputs from processor -> Send to client
	go func() {
		for {
			select {
			case text := <-textOutputChan:
				// Write JSON (Text)
				// Note: simplified for stub
				c.Conn.WriteJSON(map[string]interface{}{
					"type": "text",
					"data": text,
				})
			case audio := <-audioOutputChan:
				// Write Binary (Audio)
				c.Conn.WriteMessage(websocket.BinaryMessage, audio)
			}
		}
	}()

	for {
		msgType, msg, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		if msgType == websocket.BinaryMessage {
			if len(msg) > 0 {
				dataType := msg[0]
				data := msg[1:]

				switch dataType {
				case 1:
					select {
					case videoFrameChan <- data:
					default:
					}
				case 2:
					select {
					case audioChunkChan <- data:
					default:
					}
				}
			}
		}
	}

	close(videoFrameChan)
	close(audioChunkChan)
}

func (c *Client) WritePump() {
	for message := range c.Send {
		c.Conn.WriteMessage(websocket.TextMessage, message)
	}
	c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
}
