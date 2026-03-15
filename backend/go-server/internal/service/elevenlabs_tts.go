package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// ElevenLabsConfig holds ElevenLabs API configuration
type ElevenLabsConfig struct {
	APIKey  string
	VoiceID string // Default voice ID to use
	ModelID string // Model ID (e.g., "eleven_multilingual_v2")
}

// VoiceSettings configures the voice output characteristics
type VoiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style,omitempty"`
	UseSpeakerBoost bool    `json:"use_speaker_boost,omitempty"`
}

// TTSRequest represents a text-to-speech request
type TTSRequest struct {
	Text          string        `json:"text"`
	ModelID       string        `json:"model_id"`
	VoiceSettings VoiceSettings `json:"voice_settings"`
}

// ElevenLabsTTS handles text-to-speech using ElevenLabs API
type ElevenLabsTTS struct {
	config     ElevenLabsConfig
	httpClient *http.Client
	mu         sync.RWMutex

	// Voice cache for available voices
	voiceCache map[string]string // voice name -> voice ID
}

// Voice represents an ElevenLabs voice
type Voice struct {
	VoiceID string `json:"voice_id"`
	Name    string `json:"name"`
}

// VoicesResponse represents the response from voices API
type VoicesResponse struct {
	Voices []Voice `json:"voices"`
}

// NewElevenLabsTTS creates a new ElevenLabs TTS service
func NewElevenLabsTTS(apiKey string) *ElevenLabsTTS {
	return &ElevenLabsTTS{
		config: ElevenLabsConfig{
			APIKey:  apiKey,
			VoiceID: "21m00Tcm4TlvDq8ikWAM",   // Rachel - default female voice
			ModelID: "eleven_multilingual_v2", // Best multilingual model
		},
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		voiceCache: make(map[string]string),
	}
}

// IsConfigured returns true if API key is set
func (e *ElevenLabsTTS) IsConfigured() bool {
	return e.config.APIKey != ""
}

// SetVoice sets the voice to use for TTS
func (e *ElevenLabsTTS) SetVoice(voiceID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config.VoiceID = voiceID
}

// SetModel sets the model to use
func (e *ElevenLabsTTS) SetModel(modelID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config.ModelID = modelID
}

// SynthesizeSpeech converts text to speech and returns audio bytes (MP3 format)
func (e *ElevenLabsTTS) SynthesizeSpeech(text string) ([]byte, error) {
	if e.config.APIKey == "" {
		return nil, fmt.Errorf("ElevenLabs API key not configured")
	}

	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	e.mu.RLock()
	voiceID := e.config.VoiceID
	modelID := e.config.ModelID
	e.mu.RUnlock()

	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voiceID)

	reqBody := TTSRequest{
		Text:    text,
		ModelID: modelID,
		VoiceSettings: VoiceSettings{
			Stability:       0.5,
			SimilarityBoost: 0.75,
			Style:           0.0,
			UseSpeakerBoost: true,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "audio/mpeg")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", e.config.APIKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ElevenLabs API error (status %d): %s", resp.StatusCode, string(body))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("ElevenLabs: Synthesized %d bytes of audio for text: %s", len(audioData), truncateText(text, 50))
	return audioData, nil
}

// SynthesizeSpeechPCM converts text to speech and returns PCM audio (for telephony)
// Returns mulaw 8kHz audio suitable for Twilio
func (e *ElevenLabsTTS) SynthesizeSpeechPCM(text string) ([]byte, error) {
	if e.config.APIKey == "" {
		return nil, fmt.Errorf("ElevenLabs API key not configured")
	}

	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	e.mu.RLock()
	voiceID := e.config.VoiceID
	modelID := e.config.ModelID
	e.mu.RUnlock()

	// Use PCM output format for telephony compatibility
	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s?output_format=ulaw_8000", voiceID)

	reqBody := TTSRequest{
		Text:    text,
		ModelID: modelID,
		VoiceSettings: VoiceSettings{
			Stability:       0.5,
			SimilarityBoost: 0.75,
			Style:           0.0,
			UseSpeakerBoost: true,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "audio/basic") // mulaw format
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", e.config.APIKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ElevenLabs API error (status %d): %s", resp.StatusCode, string(body))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("ElevenLabs: Synthesized %d bytes of PCM audio for text: %s", len(audioData), truncateText(text, 50))
	return audioData, nil
}

// GetVoices retrieves available voices from ElevenLabs
func (e *ElevenLabsTTS) GetVoices() ([]Voice, error) {
	if e.config.APIKey == "" {
		return nil, fmt.Errorf("ElevenLabs API key not configured")
	}

	req, err := http.NewRequest("GET", "https://api.elevenlabs.io/v1/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("xi-api-key", e.config.APIKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ElevenLabs API error (status %d): %s", resp.StatusCode, string(body))
	}

	var voicesResp VoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&voicesResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Update cache
	e.mu.Lock()
	for _, v := range voicesResp.Voices {
		e.voiceCache[v.Name] = v.VoiceID
	}
	e.mu.Unlock()

	log.Printf("ElevenLabs: Retrieved %d voices", len(voicesResp.Voices))
	return voicesResp.Voices, nil
}

// GetVoiceIDByName returns voice ID for a given voice name
func (e *ElevenLabsTTS) GetVoiceIDByName(name string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	id, exists := e.voiceCache[name]
	return id, exists
}

// Common ElevenLabs voice IDs for quick reference
const (
	VoiceRachel = "21m00Tcm4TlvDq8ikWAM" // Female, American
	VoiceDomi   = "AZnzlk1XvdvUeBnXmlld" // Female, American
	VoiceBella  = "EXAVITQu4vr4xnSDxMaL" // Female, American
	VoiceAntoni = "ErXwobaYiN019PkySvjV" // Male, American
	VoiceElly   = "MF3mGyEYCl7XYWbV9V6O" // Female, American
	VoiceJosh   = "TxGEqnHWrfWFTfGW9XjX" // Male, American
	VoiceArnold = "VR6AewLTigWG4xSOukaG" // Male, American
	VoiceAdam   = "pNInz6obpgDQGcFmaJgB" // Male, American
	VoiceSam    = "yoZ06aMxZJJ28mfd3POQ" // Male, American
)

// truncateText truncates text for logging
func truncateText(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}
