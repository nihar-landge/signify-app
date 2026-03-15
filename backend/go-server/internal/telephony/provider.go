package telephony

import (
	"context"
)

// TelephonyProvider defines the interface for different PSTN providers
type TelephonyProvider interface {
	// Call management
	PlaceCall(ctx context.Context, toNumber string) (*Call, error)
	PlaceCallWithSession(ctx context.Context, sessionID, toNumber string) (*Call, error)
	EndCall(ctx context.Context, callSID string) error
	GetCall(callSID string) (*Call, bool)
	GetActiveCalls() []*Call

	// Audio handling
	InjectAudio(callSID string, audioData []byte) error
	HandleAudioStream(callSID string, audioData []byte)

	// Webhook handling
	HandleCallStatusWebhook(callSID string, status string)

	// Callbacks
	SetCallAnsweredCallback(cb func(callSID string))
	SetCallEndedCallback(cb func(callSID string, duration int))
	SetAudioReceivedCallback(cb func(callSID string, audio []byte))
}
