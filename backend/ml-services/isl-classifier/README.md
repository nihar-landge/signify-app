# ISL Classifier gRPC Service

A standalone gRPC service for **Indian Sign Language (ISL)** recognition using an ONNX model converted from the [SignAI](https://github.com/CUH-MCA/SignAI-Sign-Language-Recognition-and-Translation) project.

## Features

- 🇮🇳 **39 ISL signs**: Alphabets (a-z), numerals (0-9), and special symbols
- ⚡ **Fast inference**: ~3ms per prediction using ONNX Runtime
- 📡 **Multiple input formats**: Hand landmarks or MediaPipe Holistic
- 🔄 **Streaming support**: Real-time continuous recognition
- 🐳 **Docker ready**: Easy containerized deployment

## Quick Start

### Local Development

```bash
# Install dependencies
pip install -r requirements.txt

# Generate proto stubs (if not already)
python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. isl_classifier.proto

# Start the service
python server.py --port 50054

# Test the service
python test_client.py
```

### Docker

```bash
# Build
docker build -t signify/isl-classifier .

# Run
docker run -p 50054:50054 signify/isl-classifier
```

## API Reference

### Endpoints

| Method | Description |
|--------|-------------|
| `RecognizeLandmarks` | Recognize ISL from hand landmarks (84 floats) |
| `RecognizeHolistic` | Recognize ISL from MediaPipe Holistic (543 points) |
| `StreamRecognize` | Streaming recognition for real-time processing |
| `GetLabels` | Get all 39 available ISL labels |
| `HealthCheck` | Service health status |

### Input Format

**Hand Landmarks** (84 floats):
- Left hand: 21 points × 2 coords (x, y) = 42 floats
- Right hand: 21 points × 2 coords (x, y) = 42 floats
- Wrist-relative, max-normalized

**Holistic Landmarks** (1629 floats):
- 543 points × 3 coords (x, y, z)
- Order: Pose (33) + Face (468) + LeftHand (21) + RightHand (21)

### Example Client (Python)

```python
import grpc
import isl_classifier_pb2
import isl_classifier_pb2_grpc

# Connect
channel = grpc.insecure_channel('localhost:50054')
stub = isl_classifier_pb2_grpc.ISLClassifierStub(channel)

# Recognize from hand landmarks
response = stub.RecognizeLandmarks(
    isl_classifier_pb2.LandmarksRequest(
        session_id="user-123",
        left_hand=[...],  # 42 floats
        right_hand=[...],  # 42 floats
        timestamp=12345
    )
)

print(f"Sign: {response.label} ({response.confidence:.0%})")
```

## Labels

The model recognizes 39 ISL signs:

| Category | Labels |
|----------|--------|
| Alphabets | a, b, c, d, e, f, g, h, i, j, k, l, m, n, o, p, q, r, s, t, u, v, w, x, y, z |
| Numerals | 0, 1, 2, 3, 4, 5, 6, 7, 8, 9 |
| Special | ~, !, fullstop |

## Model Architecture

```
Input (84) → Dense(42, ReLU) → Dense(10, ReLU) → Dense(20, ReLU) → Dense(39, Softmax)
```

Converted from SignAI's HDF5 model to ONNX format.

## Files

```
isl-classifier/
├── server.py                 # Main gRPC service
├── isl_classifier.proto      # Proto definitions
├── isl_classifier_pb2.py     # Generated Python stubs
├── isl_classifier_pb2_grpc.py
├── test_client.py            # Test client
├── requirements.txt          # Dependencies
├── Dockerfile               # Container config
└── models/
    ├── signai_isl_classifier.onnx  # ONNX model
    └── signai_isl_labels.json      # Label mapping
```

## Integration with Go Backend

Add to your Go gRPC client:

```go
import "github.com/signify/backend/proto/isl_classifier"

// Connect
conn, _ := grpc.Dial("localhost:50054", grpc.WithInsecure())
client := isl_classifier.NewISLClassifierClient(conn)

// Recognize
resp, _ := client.RecognizeLandmarks(ctx, &isl_classifier.LandmarksRequest{
    SessionId: "user-123",
    LeftHand:  leftHandFloats,
    RightHand: rightHandFloats,
})
```

## License

Based on the [SignAI](https://github.com/CUH-MCA/SignAI-Sign-Language-Recognition-and-Translation) project.
