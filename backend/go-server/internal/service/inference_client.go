package service

import (
	"context"
	"math"
	"time"

	pb "signify/internal/grpc/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type InferenceClient struct {
	conn   *grpc.ClientConn
	client pb.SignInferenceClient
}

func NewInferenceClient(addr string) (*InferenceClient, error) {
	// grpc.Dial is deprecated, use NewClient
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &InferenceClient{
		conn:   conn,
		client: pb.NewSignInferenceClient(conn),
	}, nil
}

func (c *InferenceClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

type Prediction struct {
	Label      string
	Confidence float32
	ClassIndex int
}

func (c *InferenceClient) Predict(sessionID string, frames [][]float32) ([]Prediction, error) {
	// 500ms timeout for real-time inference
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if len(frames) == 0 {
		return nil, nil
	}

	// 1. Flatten frames to byte buffer
	// Total floats = frames * landmarks * coords
	// Assuming each frame is 543*3 = 1629 floats
	totalFloats := 0
	for _, f := range frames {
		totalFloats += len(f)
	}

	// Create byte buffer (each float32 is 4 bytes)
	// We need to write them in Little Endian format (standard for passing to Python/numpy usually)
	// But simpler approach: create a flat []float32 first, then cast to bytes?
	// Go doesn't let you cast slice types easily without unsafe.
	// We can use encoding/binary.

	// Create byte buffer (each float32 is 4 bytes)
	tensorBytes := make([]byte, totalFloats*4)

	offset := 0
	for _, frame := range frames {
		for _, val := range frame {
			// Convert float32 to uint32 bits
			bits := math.Float32bits(val)
			// Write 4 bytes (Little Endian is standard for x86/ARM)
			tensorBytes[offset] = byte(bits)
			tensorBytes[offset+1] = byte(bits >> 8)
			tensorBytes[offset+2] = byte(bits >> 16)
			tensorBytes[offset+3] = byte(bits >> 24)
			offset += 4
		}
	}

	numFrames := int32(len(frames))
	numLandmarks := int32(543)
	numCoords := int32(3)

	req := &pb.PredictRequest{
		SessionId:     sessionID,
		TensorContent: tensorBytes,
		Shape:         []int32{numFrames, numLandmarks, numCoords},
	}

	resp, err := c.client.Predict(ctx, req)
	if err != nil {
		return nil, err
	}

	var predictions []Prediction
	for _, p := range resp.TopK {
		predictions = append(predictions, Prediction{
			Label:      p.Label,
			Confidence: p.Confidence,
			ClassIndex: int(p.ClassIndex),
		})
	}
	return predictions, nil
}
