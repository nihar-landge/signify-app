package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	SignToTextURL  string
	TextToVoiceURL string
	PostgresURL    string

	// Twilio configuration
	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioFromNumber string

	// Public URLs for Twilio webhooks (must be HTTPS/WSS accessible)
	PublicBaseURL string // e.g., https://xxxx.ngrok-free.app
	PublicWSURL   string // e.g., wss://xxxx.ngrok-free.app

	// Deepgram STT configuration
	DeepgramAPIKey string

	// ElevenLabs TTS configuration
	ElevenLabsAPIKey  string
	ElevenLabsVoiceID string // Optional: default voice ID to use

	// Exotel configuration
	ExotelSID   string
	ExotelKey   string
	ExotelToken string

	// Test Mode configuration
	AllowTestSessions bool
}

func Load() *Config {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("Note: No .env file found, using system environment variables")
	}

	cfg := &Config{
		Port:           getEnv("PORT", "8080"),
		SignToTextURL:  getEnv("SIGN_TO_TEXT_URL", "localhost:50051"),
		TextToVoiceURL: getEnv("TEXT_TO_VOICE_URL", "localhost:50052"),
		PostgresURL:    getEnv("POSTGRES_URL", "postgres://user:pass@localhost:5432/signify?sslmode=disable"),

		// Twilio config - supports both naming conventions
		TwilioAccountSID: getEnv("TWILIO_ACCOUNT_SID", ""),
		TwilioAuthToken:  getEnv("TWILIO_AUTH_TOKEN", ""),
		TwilioFromNumber: getEnvWithFallback("TWILIO_FROM_NUMBER", "TWILIO_PHONE_NUMBER", ""),

		// Public URLs
		PublicBaseURL: getEnv("PUBLIC_BASE_URL", ""),
		PublicWSURL:   getEnv("PUBLIC_WS_URL", ""),

		// Deepgram
		DeepgramAPIKey: getEnv("DEEPGRAM_API_KEY", ""),

		// ElevenLabs
		ElevenLabsAPIKey:  getEnv("ELEVENLABS_API_KEY", ""),
		ElevenLabsVoiceID: getEnv("ELEVENLABS_VOICE_ID", "21m00Tcm4TlvDq8ikWAM"), // Rachel voice

		// Exotel
		ExotelSID:   getEnv("EXOTEL_SID", ""),
		ExotelKey:   getEnv("EXOTEL_KEY", ""),
		ExotelToken: getEnv("EXOTEL_TOKEN", ""),

		// Test Mode
		AllowTestSessions: strings.EqualFold(getEnv("ALLOW_TEST_SESSIONS", "false"), "true"),
	}

	// Validate required config for Twilio integration
	cfg.validate()

	return cfg
}

func (c *Config) validate() {
	// Warn if Twilio is partially configured
	hasTwilioCreds := c.TwilioAccountSID != "" && c.TwilioAuthToken != ""
	hasPublicURLs := c.PublicBaseURL != "" && c.PublicWSURL != ""

	if hasTwilioCreds && !hasPublicURLs {
		log.Println("WARNING: Twilio credentials set but PUBLIC_BASE_URL/PUBLIC_WS_URL missing. Twilio webhooks will fail.")
	}

	if hasPublicURLs && !hasTwilioCreds {
		log.Println("WARNING: Public URLs set but Twilio credentials missing. Calls will be simulated.")
	}

	if !hasTwilioCreds {
		log.Println("INFO: Twilio credentials not configured. PSTN calls will be simulated.")
	}

	if hasTwilioCreds && hasPublicURLs {
		log.Printf("INFO: Twilio configured with public URL: %s", c.PublicBaseURL)
	}

	// Deepgram validation
	if c.DeepgramAPIKey == "" {
		log.Println("INFO: Deepgram API key not configured. STT will be disabled.")
	} else {
		log.Println("INFO: Deepgram STT configured.")
	}

	// ElevenLabs validation
	if c.ElevenLabsAPIKey == "" {
		log.Println("INFO: ElevenLabs API key not configured. TTS will use fallback.")
	} else {
		log.Printf("INFO: ElevenLabs TTS configured with voice: %s", c.ElevenLabsVoiceID)
	}

	// Exotel validation
	if c.ExotelSID == "" || c.ExotelKey == "" || c.ExotelToken == "" {
		log.Println("INFO: Exotel credentials not fully configured. Indian numbers may not be available.")
	} else {
		log.Println("INFO: Exotel configured.")
	}
}

// IsTwilioConfigured returns true if all Twilio settings are present
func (c *Config) IsTwilioConfigured() bool {
	return c.TwilioAccountSID != "" &&
		c.TwilioAuthToken != "" &&
		c.TwilioFromNumber != "" &&
		c.PublicBaseURL != "" &&
		c.PublicWSURL != ""
}

// IsDeepgramConfigured returns true if Deepgram API key is set
func (c *Config) IsDeepgramConfigured() bool {
	return c.DeepgramAPIKey != ""
}

// IsElevenLabsConfigured returns true if ElevenLabs API key is set
func (c *Config) IsElevenLabsConfigured() bool {
	return c.ElevenLabsAPIKey != ""
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// getEnvWithFallback tries primary key first, then fallback key
func getEnvWithFallback(primary, fallbackKey, defaultVal string) string {
	if value, ok := os.LookupEnv(primary); ok {
		return value
	}
	if value, ok := os.LookupEnv(fallbackKey); ok {
		return value
	}
	return defaultVal
}
