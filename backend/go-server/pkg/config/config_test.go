package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear any env vars that would override defaults
	os.Unsetenv("PORT")
	os.Unsetenv("SIGN_TO_TEXT_URL")
	os.Unsetenv("TEXT_TO_VOICE_URL")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected default port '8080', got '%s'", cfg.Port)
	}
	if cfg.SignToTextURL != "localhost:50051" {
		t.Errorf("expected default SignToTextURL 'localhost:50051', got '%s'", cfg.SignToTextURL)
	}
	if cfg.TextToVoiceURL != "localhost:50052" {
		t.Errorf("expected default TextToVoiceURL 'localhost:50052', got '%s'", cfg.TextToVoiceURL)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("SIGN_TO_TEXT_URL", "ml-server:50051")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("SIGN_TO_TEXT_URL")

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("expected port '9090', got '%s'", cfg.Port)
	}
	if cfg.SignToTextURL != "ml-server:50051" {
		t.Errorf("expected SignToTextURL 'ml-server:50051', got '%s'", cfg.SignToTextURL)
	}
}

func TestIsTwilioConfigured(t *testing.T) {
	cfg := &Config{}
	if cfg.IsTwilioConfigured() {
		t.Error("expected Twilio to not be configured with empty config")
	}

	cfg = &Config{
		TwilioAccountSID: "AC123",
		TwilioAuthToken:  "token123",
		TwilioFromNumber: "+1234567890",
		PublicBaseURL:    "https://example.com",
		PublicWSURL:      "wss://example.com",
	}
	if !cfg.IsTwilioConfigured() {
		t.Error("expected Twilio to be configured with full config")
	}
}

func TestIsDeepgramConfigured(t *testing.T) {
	cfg := &Config{}
	if cfg.IsDeepgramConfigured() {
		t.Error("expected Deepgram to not be configured with empty config")
	}

	cfg = &Config{DeepgramAPIKey: "key123"}
	if !cfg.IsDeepgramConfigured() {
		t.Error("expected Deepgram to be configured with API key")
	}
}

func TestIsElevenLabsConfigured(t *testing.T) {
	cfg := &Config{}
	if cfg.IsElevenLabsConfigured() {
		t.Error("expected ElevenLabs to not be configured with empty config")
	}

	cfg = &Config{ElevenLabsAPIKey: "key123"}
	if !cfg.IsElevenLabsConfigured() {
		t.Error("expected ElevenLabs to be configured with API key")
	}
}
