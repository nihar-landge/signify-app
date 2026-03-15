package service

import (
	"context"
	"log"
	"signify/pkg/config"
	"signify/pkg/grpc_client/pb"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type StreamProcessor struct {
	signClient     pb.SignRecognitionClient
	ttsClient      pb.TextToVoiceClient
	sessionManager *SessionManager
}

func NewStreamProcessor(cfg *config.Config) *StreamProcessor {
	// Initialize gRPC clients with non-blocking connections
	// Connections will be established lazily when first used

	sp := &StreamProcessor{
		sessionManager: NewSessionManager(nil),
	}

	// 1. Sign Client
	connSign, err := grpc.Dial(cfg.SignToTextURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(2*time.Second),
	)
	if err != nil {
		log.Printf("Warning: Sign Service not available at %s: %v", cfg.SignToTextURL, err)
	} else {
		sp.signClient = pb.NewSignRecognitionClient(connSign)
		log.Printf("Connected to Sign Service at %s", cfg.SignToTextURL)
	}

	// 2. TTS Client
	connTTS, err := grpc.Dial(cfg.TextToVoiceURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(2*time.Second),
	)
	if err != nil {
		log.Printf("Warning: TTS Service not available at %s: %v", cfg.TextToVoiceURL, err)
	} else {
		sp.ttsClient = pb.NewTextToVoiceClient(connTTS)
		log.Printf("Connected to TTS Service at %s", cfg.TextToVoiceURL)
	}

	// Note: STT is handled by Deepgram streaming (see stt_service.go)
	// No separate gRPC STT microservice needed

	return sp
}

// ProcessVideoStream handles video frames from camera for Sign -> Text -> Voice
func (sp *StreamProcessor) ProcessVideoStream(
	userID string,
	videoFrameChan <-chan []byte,
	textOutputChan chan<- string,
	audioOutputChan chan<- []byte,
) {
	for frame := range videoFrameChan {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		// 1. Sign -> Text
		resp, err := sp.signClient.RecognizeSign(ctx, &pb.RecognitionRequest{
			ImageData: frame,
			Timestamp: time.Now().Unix(),
		})

		if err != nil {
			log.Printf("Sign Recog Error: %v", err)
			cancel()
			continue
		}

		if resp.Text != "" {
			// Send Text to client
			textOutputChan <- resp.Text

			// Save to session
			sp.sessionManager.SaveMessage(userID, "user", resp.Text)

			// 2. Text -> Voice
			ttsResp, err := sp.ttsClient.Synthesize(ctx, &pb.SynthesisRequest{
				Text: resp.Text,
			})

			if err != nil {
				log.Printf("TTS Error: %v", err)
			} else {
				audioOutputChan <- ttsResp.AudioData
			}
		}

		cancel()
	}
}

// ProcessMicrophoneStream handles audio from microphone for Voice -> Text
func (sp *StreamProcessor) ProcessMicrophoneStream(
	userID string,
	audioChunkChan <-chan []byte,
	textOutputChan chan<- string,
) {
	// STT is handled by Deepgram streaming (see stt_service.go / startSTTWorker)
	// This method is a legacy stub — real-time STT goes through the CallHandler pipeline
	for range audioChunkChan {
		// Audio chunks are routed to Deepgram via the CallHandler STT pipeline
	}
}

// ProcessCallStream handles audio during phone calls for real-time captions
func (sp *StreamProcessor) ProcessCallStream(
	userID string,
	callAudioChan <-chan []byte,
	captionOutputChan chan<- string,
	ttsOutputChan chan<- []byte,
) {
	// STT is handled by Deepgram streaming (see stt_service.go / startSTTWorker)
	// This method is a legacy stub — real-time STT goes through the CallHandler pipeline
	for range callAudioChan {
		// Audio chunks are routed to Deepgram via the CallHandler STT pipeline
	}
}

// BatchFrames batches video frames to reduce model calls and improve performance
func (sp *StreamProcessor) BatchFrames(
	inputChan <-chan []byte,
	batchSize int,
) <-chan [][]byte {
	outputChan := make(chan [][]byte)

	go func() {
		batch := make([][]byte, 0, batchSize)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case frame, ok := <-inputChan:
				if !ok {
					// Channel closed, flush remaining batch
					if len(batch) > 0 {
						outputChan <- batch
					}
					close(outputChan)
					return
				}
				batch = append(batch, frame)
				if len(batch) >= batchSize {
					outputChan <- batch
					batch = make([][]byte, 0, batchSize)
				}
			case <-ticker.C:
				if len(batch) > 0 {
					outputChan <- batch
					batch = make([][]byte, 0, batchSize)
				}
			}
		}
	}()

	return outputChan
}

// GetSessionManager returns the session manager for external access
func (sp *StreamProcessor) GetSessionManager() *SessionManager {
	return sp.sessionManager
}
