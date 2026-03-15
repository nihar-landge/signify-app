package service

import (
	"context"
	"encoding/json"
	"log"
	"signify/pkg/config"
	"signify/pkg/grpc_client/pb"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// LandmarkFrame represents a single frame of landmark data
type LandmarkFrame struct {
	Timestamp int64       `json:"ts"`
	Sequence  int         `json:"seq"`
	LeftHand  [][]float64 `json:"lh,omitempty"`   // 21 points, each [x, y, z]
	RightHand [][]float64 `json:"rh,omitempty"`   // 21 points, each [x, y, z]
	Pose      [][]float64 `json:"pose,omitempty"` // 33 points, each [x, y, z]
	Face      [][]float64 `json:"face,omitempty"` // Key face points
}

// TranslationResult represents the output from sign translation
type TranslationResult struct {
	Text       string
	IsFinal    bool
	Confidence float32
	Timestamp  int64
}

// LandmarkBuffer holds a rolling window of landmark frames
type LandmarkBuffer struct {
	frames    []LandmarkFrame
	maxFrames int
	mu        sync.Mutex
}

// NewLandmarkBuffer creates a new buffer with the specified capacity
func NewLandmarkBuffer(maxFrames int) *LandmarkBuffer {
	return &LandmarkBuffer{
		frames:    make([]LandmarkFrame, 0, maxFrames),
		maxFrames: maxFrames,
	}
}

// Add adds a frame to the buffer
func (lb *LandmarkBuffer) Add(frame LandmarkFrame) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.frames = append(lb.frames, frame)

	// Keep only the last maxFrames
	if len(lb.frames) > lb.maxFrames {
		lb.frames = lb.frames[len(lb.frames)-lb.maxFrames:]
	}
}

// GetAll returns all frames in the buffer
func (lb *LandmarkBuffer) GetAll() []LandmarkFrame {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	result := make([]LandmarkFrame, len(lb.frames))
	copy(result, lb.frames)
	return result
}

// Clear empties the buffer
func (lb *LandmarkBuffer) Clear() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.frames = lb.frames[:0]
}

// LandmarkProcessor handles continuous sign language recognition from landmarks
type LandmarkProcessor struct {
	signClient pb.SignRecognitionClient
	ttsClient  pb.TextToVoiceClient

	// Per-session buffers
	buffers  map[string]*LandmarkBuffer
	bufferMu sync.RWMutex

	// Processing configuration
	windowDuration  time.Duration // How much data to send to model
	processInterval time.Duration // How often to run translation

	// Debounce settings
	lastResults map[string]string // Last result per session for dedup
	resultMu    sync.RWMutex
}

// NewLandmarkProcessor creates a new landmark processor
func NewLandmarkProcessor(cfg *config.Config) *LandmarkProcessor {
	lp := &LandmarkProcessor{
		buffers:         make(map[string]*LandmarkBuffer),
		windowDuration:  2 * time.Second,        // 2 second rolling window
		processInterval: 300 * time.Millisecond, // Process every 300ms
		lastResults:     make(map[string]string),
	}

	// Connect to Sign Recognition service
	connSign, err := grpc.Dial(cfg.SignToTextURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(2*time.Second),
	)
	if err != nil {
		log.Printf("Warning: Sign Service not available at %s: %v", cfg.SignToTextURL, err)
	} else {
		lp.signClient = pb.NewSignRecognitionClient(connSign)
		log.Printf("LandmarkProcessor connected to Sign Service at %s", cfg.SignToTextURL)
	}

	// Connect to TTS service
	connTTS, err := grpc.Dial(cfg.TextToVoiceURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(2*time.Second),
	)
	if err != nil {
		log.Printf("Warning: TTS Service not available at %s: %v", cfg.TextToVoiceURL, err)
	} else {
		lp.ttsClient = pb.NewTextToVoiceClient(connTTS)
		log.Printf("LandmarkProcessor connected to TTS Service at %s", cfg.TextToVoiceURL)
	}

	return lp
}

// StartSession begins processing for a new session
func (lp *LandmarkProcessor) StartSession(sessionID string) {
	lp.bufferMu.Lock()
	lp.buffers[sessionID] = NewLandmarkBuffer(60) // ~2 seconds at 30fps
	lp.bufferMu.Unlock()

	log.Printf("Started landmark processing for session %s", sessionID)
}

// EndSession stops processing for a session
func (lp *LandmarkProcessor) EndSession(sessionID string) {
	lp.bufferMu.Lock()
	delete(lp.buffers, sessionID)
	lp.bufferMu.Unlock()

	lp.resultMu.Lock()
	delete(lp.lastResults, sessionID)
	lp.resultMu.Unlock()

	log.Printf("Ended landmark processing for session %s", sessionID)
}

// AddFrame adds a landmark frame to the session's buffer
func (lp *LandmarkProcessor) AddFrame(sessionID string, data []byte) error {
	lp.bufferMu.RLock()
	buffer, exists := lp.buffers[sessionID]
	lp.bufferMu.RUnlock()

	if !exists {
		return nil
	}

	var frame LandmarkFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return err
	}

	buffer.Add(frame)
	return nil
}

// ProcessContinuous runs continuous translation for a session
func (lp *LandmarkProcessor) ProcessContinuous(
	ctx context.Context,
	sessionID string,
	textOutput chan<- TranslationResult,
	audioOutput chan<- []byte,
) {
	ticker := time.NewTicker(lp.processInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lp.bufferMu.RLock()
			buffer, exists := lp.buffers[sessionID]
			lp.bufferMu.RUnlock()

			if !exists {
				return
			}

			frames := buffer.GetAll()
			if len(frames) < 5 { // Need at least some frames
				continue
			}

			// Translate the buffered landmarks
			result, err := lp.translate(ctx, frames)
			if err != nil {
				log.Printf("Translation error: %v", err)
				continue
			}

			if result.Text == "" {
				continue
			}

			// Debounce: don't repeat the same result
			lp.resultMu.RLock()
			lastResult := lp.lastResults[sessionID]
			lp.resultMu.RUnlock()

			if result.Text == lastResult && !result.IsFinal {
				continue
			}

			lp.resultMu.Lock()
			lp.lastResults[sessionID] = result.Text
			lp.resultMu.Unlock()

			// Send text result
			select {
			case textOutput <- result:
			default:
			}

			// If final result, generate TTS and clear buffer
			if result.IsFinal {
				buffer.Clear()

				audio, err := lp.GenerateTTS(ctx, result.Text)
				if err != nil {
					log.Printf("TTS error: %v", err)
					continue
				}

				select {
				case audioOutput <- audio:
				default:
				}
			}
		}
	}
}

// translate sends landmarks to the sign recognition service
func (lp *LandmarkProcessor) translate(ctx context.Context, frames []LandmarkFrame) (TranslationResult, error) {
	if lp.signClient == nil || len(frames) == 0 {
		// Fallback: simple gesture detection from landmarks
		return lp.fallbackTranslate(frames), nil
	}

	// Use the most recent frame for single-frame classification
	lastFrame := frames[len(frames)-1]
	landmarks := lp.flattenSingleFrame(lastFrame)

	// Need at least some landmarks to make a prediction
	if len(landmarks) < 100 {
		return lp.fallbackTranslate(frames), nil
	}

	// Make gRPC call with timeout
	callCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	resp, err := lp.signClient.RecognizeHolistic(callCtx, &pb.HolisticRequest{
		SessionId: "default",
		Landmarks: landmarks,
		Timestamp: lastFrame.Timestamp,
	})
	if err != nil {
		log.Printf("gRPC sign recognition error: %v, falling back to heuristic", err)
		return lp.fallbackTranslate(frames), nil
	}

	// Only return result if confidence is reasonable
	if resp.Confidence < 0.3 || resp.Text == "" {
		return TranslationResult{}, nil
	}

	log.Printf("ONNX prediction: '%s' (confidence: %.2f)", resp.Text, resp.Confidence)

	return TranslationResult{
		Text:       resp.Text,
		Confidence: resp.Confidence,
		IsFinal:    resp.Confidence > 0.7,
		Timestamp:  lastFrame.Timestamp,
	}, nil
}

// flattenSingleFrame converts a single LandmarkFrame to a flat []float32 for ONNX model input
// Order: Pose (33×3) + Face (468×3) + LeftHand (21×3) + RightHand (21×3) = 543×3 = 1629 floats
func (lp *LandmarkProcessor) flattenSingleFrame(frame LandmarkFrame) []float32 {
	var result []float32

	// Pose landmarks (33 points × 3 coords = 99 floats)
	for _, pt := range frame.Pose {
		for _, v := range pt {
			result = append(result, float32(v))
		}
	}

	// Face landmarks (468 points × 3 coords = 1404 floats)
	for _, pt := range frame.Face {
		for _, v := range pt {
			result = append(result, float32(v))
		}
	}

	// Left Hand landmarks (21 points × 3 coords = 63 floats)
	for _, pt := range frame.LeftHand {
		for _, v := range pt {
			result = append(result, float32(v))
		}
	}

	// Right Hand landmarks (21 points × 3 coords = 63 floats)
	for _, pt := range frame.RightHand {
		for _, v := range pt {
			result = append(result, float32(v))
		}
	}

	return result
}

// flattenFrames converts frames to a flat array for model input
func (lp *LandmarkProcessor) flattenFrames(frames []LandmarkFrame) []float64 {
	// Flatten all landmark points across all frames
	var result []float64

	for _, frame := range frames {
		// Add pose landmarks
		for _, point := range frame.Pose {
			result = append(result, point...)
		}

		// Add hand landmarks
		for _, point := range frame.LeftHand {
			result = append(result, point...)
		}
		for _, point := range frame.RightHand {
			result = append(result, point...)
		}
	}

	return result
}

// fallbackTranslate provides simple gesture detection without ML model
func (lp *LandmarkProcessor) fallbackTranslate(frames []LandmarkFrame) TranslationResult {
	if len(frames) == 0 {
		return TranslationResult{}
	}

	// Simple heuristic-based gesture detection
	// Check the most recent frames for patterns
	lastFrame := frames[len(frames)-1]

	// Check if hands are visible (have pose data with wrist positions)
	hasHandMotion := len(lastFrame.Pose) > 0

	if !hasHandMotion {
		return TranslationResult{}
	}

	// Very basic detection based on pose position
	// In production, this would use a trained LSTM/Transformer model

	// Detect if hands are raised (wrist above a threshold)
	if len(lastFrame.Pose) >= 16 {
		leftWrist := lastFrame.Pose[15]  // PoseLandmark.LEFT_WRIST
		rightWrist := lastFrame.Pose[16] // PoseLandmark.RIGHT_WRIST

		// Check if both hands are raised (y < 0.3, normalized)
		if len(leftWrist) >= 2 && len(rightWrist) >= 2 {
			if leftWrist[1] < 0.3 && rightWrist[1] < 0.3 {
				return TranslationResult{
					Text:       "Hello",
					IsFinal:    true,
					Confidence: 0.75,
					Timestamp:  lastFrame.Timestamp,
				}
			}

			// One hand raised
			if leftWrist[1] < 0.4 || rightWrist[1] < 0.4 {
				return TranslationResult{
					Text:       "Hi",
					IsFinal:    true,
					Confidence: 0.65,
					Timestamp:  lastFrame.Timestamp,
				}
			}
		}
	}

	return TranslationResult{}
}

// GenerateTTS converts text to speech audio
func (lp *LandmarkProcessor) GenerateTTS(ctx context.Context, text string) ([]byte, error) {
	if lp.ttsClient == nil {
		log.Printf("TTS not available, skipping audio generation for: %s", text)
		return nil, nil
	}

	resp, err := lp.ttsClient.Synthesize(ctx, &pb.SynthesisRequest{
		Text: text,
	})
	if err != nil {
		return nil, err
	}

	return resp.AudioData, nil
}
