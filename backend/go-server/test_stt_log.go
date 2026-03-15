package main

import (
	"log"
	"time"
)

func main() {
	sessionID := "sess_8f92a1b7"
	userID := "user_4917"
	bridgeID := "BR_984fje01"
	phoneNumber := "+919876543210"

	log.Printf("Call session started: user=%s, bridge=%s, phone=%s", userID, bridgeID, phoneNumber)
	time.Sleep(1 * time.Second)

	log.Printf("STT worker starting for session %s", sessionID)
	time.Sleep(500 * time.Millisecond)

	log.Printf("STT session started: sessionId=%s", sessionID)
	time.Sleep(200 * time.Millisecond)

	log.Printf("STT worker running for session %s", sessionID)
	time.Sleep(2 * time.Second)

	log.Printf("STT final for session %s: Hello", sessionID)
	time.Sleep(2 * time.Second)

	log.Printf("STT final for session %s: I am calling to ask about your services.", sessionID)
	time.Sleep(3 * time.Second)

	log.Printf("STT final for session %s: Could you provide more details?", sessionID)
	time.Sleep(2 * time.Second)

	log.Printf("STT worker stopping (signal) for session %s", sessionID)
	time.Sleep(100 * time.Millisecond)

	log.Printf("STT session stopped: sessionId=%s", sessionID)
	time.Sleep(100 * time.Millisecond)

	log.Printf("Call session ended: user=%s", userID)
}
