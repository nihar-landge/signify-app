"""
ISL Classifier gRPC Service

A standalone gRPC service for Indian Sign Language recognition using the
SignAI ONNX model. This service provides real-time ISL alphabet, numeral,
and symbol recognition from hand landmark data.

Features:
- Support for pre-normalized hand landmarks (84-dim)
- Support for MediaPipe Holistic landmarks (543 points)
- Streaming mode for real-time recognition
- Top-K predictions with confidence scores

Architecture:
Input (84 floats) → Dense(42) → Dense(10) → Dense(20) → Dense(39) → Softmax

Runs on port 50054 by default.
"""

import grpc
from concurrent import futures
import time
import numpy as np
import json
import logging
import argparse
from pathlib import Path
import onnxruntime as ort

# Import generated proto stubs
import isl_classifier_pb2
import isl_classifier_pb2_grpc

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class ISLClassifierService(isl_classifier_pb2_grpc.ISLClassifierServicer):
    """
    gRPC Service for ISL (Indian Sign Language) recognition.
    
    Uses a trained ONNX model converted from SignAI project.
    """
    
    def __init__(self, model_path: str = None, labels_path: str = None):
        """
        Initialize the ISL classifier service.
        
        Args:
            model_path: Path to ONNX model file
            labels_path: Path to labels JSON file
        """
        # Find model and labels
        if model_path is None:
            # Look in common locations
            search_paths = [
                Path(__file__).parent / "models" / "signai_isl_classifier.onnx",
                Path(__file__).parent.parent.parent / "inference-python" / "models" / "signai_isl_classifier.onnx",
            ]
            for p in search_paths:
                if p.exists():
                    model_path = str(p)
                    break
        
        if model_path is None or not Path(model_path).exists():
            raise FileNotFoundError(
                "ONNX model not found. Please run create_signai_onnx.py first or "
                "copy models/signai_isl_classifier.onnx to this directory."
            )
        
        # Load ONNX model
        logger.info(f"Loading ONNX model from: {model_path}")
        self.session = ort.InferenceSession(model_path)
        self.input_name = self.session.get_inputs()[0].name
        self.output_name = self.session.get_outputs()[0].name
        
        # Load labels
        if labels_path is None:
            labels_path = Path(model_path).parent / "signai_isl_labels.json"
        
        if Path(labels_path).exists():
            with open(labels_path, 'r') as f:
                self.labels = json.load(f)
        else:
            # Default labels
            self.labels = [
                "~", "!", "1", "2", "3", "4", "5", "6", "8", "7", "9",
                "fullstop", "a", "b", "c", "d", "e", "f", "g", "h", "i",
                "j", "k", "l", "m", "n", "0", "o", "p", "q", "r", "s",
                "t", "u", "v", "w", "x", "y", "z"
            ]
        
        logger.info(f"Loaded {len(self.labels)} ISL labels")
        logger.info("ISL Classifier Service initialized successfully")
    
    def _normalize_hand(self, landmarks: list) -> list:
        """
        Normalize hand landmarks (wrist-relative, max-normalized).
        
        Args:
            landmarks: List of 42 floats (21 × 2) or 21 (x,y) pairs
        
        Returns:
            42-dimensional normalized vector
        """
        if len(landmarks) == 0:
            return [0.0] * 42
        
        # Handle different input formats
        if len(landmarks) == 42:
            # Already flattened
            points = [(landmarks[i*2], landmarks[i*2+1]) for i in range(21)]
        elif len(landmarks) >= 21:
            # Assume pairs or objects
            points = [(landmarks[i*2], landmarks[i*2+1]) for i in range(min(21, len(landmarks)//2))]
        else:
            return [0.0] * 42
        
        # Normalize relative to wrist
        base_x, base_y = points[0]
        normalized = [[x - base_x, y - base_y] for x, y in points]
        
        # Flatten
        flat = [coord for point in normalized for coord in point]
        
        # Max-normalize
        max_val = max(abs(v) for v in flat) if flat else 1.0
        if max_val > 0:
            flat = [v / max_val for v in flat]
        
        return flat
    
    def _preprocess_holistic(self, landmarks: list) -> np.ndarray:
        """
        Convert 543-landmark holistic input to 84-dimensional SignAI input.
        
        Args:
            landmarks: 1629 floats (543 × 3) from MediaPipe Holistic
        
        Returns:
            84-dimensional feature vector
        """
        # Reshape to (543, 3)
        if len(landmarks) == 1629:
            arr = np.array(landmarks).reshape(543, 3)
        else:
            # Try to handle other formats
            arr = np.array(landmarks)
            if len(arr.shape) == 1 and len(arr) >= 1629:
                arr = arr[:1629].reshape(543, 3)
            else:
                return np.zeros(84, dtype=np.float32)
        
        # Extract hand landmarks
        lhand_start = 33 + 468  # 501
        rhand_start = lhand_start + 21  # 522
        
        left_hand = arr[lhand_start:lhand_start + 21]
        right_hand = arr[rhand_start:rhand_start + 21]
        
        # Check validity
        left_valid = np.abs(left_hand).sum() > 0.01
        right_valid = np.abs(right_hand).sum() > 0.01
        
        # Normalize each hand
        left_norm = self._normalize_hand(
            [coord for p in left_hand[:, :2] for coord in p]
        ) if left_valid else [0.0] * 42
        
        right_norm = self._normalize_hand(
            [coord for p in right_hand[:, :2] for coord in p]
        ) if right_valid else [0.0] * 42
        
        return np.array(left_norm + right_norm, dtype=np.float32)
    
    def _predict(self, features: np.ndarray, top_k: int = 5) -> tuple:
        """
        Run inference on preprocessed features.
        
        Returns:
            (label, confidence, top_k_predictions, processing_time_ms)
        """
        start_time = time.time()
        
        # Ensure shape
        if features.ndim == 1:
            features = features.reshape(1, -1)
        
        # Run inference
        result = self.session.run(
            [self.output_name],
            {self.input_name: features.astype(np.float32)}
        )
        probs = result[0][0]
        
        # Get predictions
        top_indices = np.argsort(probs)[::-1][:top_k]
        
        top_label = self.labels[top_indices[0]]
        top_conf = float(probs[top_indices[0]])
        
        top_k_preds = [
            (self.labels[idx], float(probs[idx]))
            for idx in top_indices
        ]
        
        processing_time = (time.time() - start_time) * 1000
        
        return top_label, top_conf, top_k_preds, processing_time
    
    def RecognizeLandmarks(self, request, context):
        """
        Recognize ISL sign from normalized hand landmarks.
        """
        try:
            # Get features from request
            left_norm = self._normalize_hand(list(request.left_hand))
            right_norm = self._normalize_hand(list(request.right_hand))
            
            features = np.array(left_norm + right_norm, dtype=np.float32)
            
            # Predict
            label, confidence, top_k, proc_time = self._predict(features)
            
            logger.debug(f"Recognized: {label} ({confidence:.2%})")
            
            return isl_classifier_pb2.RecognitionResponse(
                label=label,
                confidence=confidence,
                top_k=[
                    isl_classifier_pb2.Prediction(label=l, confidence=c)
                    for l, c in top_k
                ],
                processing_time_ms=proc_time,
                session_id=request.session_id,
                timestamp=request.timestamp
            )
            
        except Exception as e:
            logger.error(f"Error in RecognizeLandmarks: {e}")
            context.set_details(str(e))
            context.set_code(grpc.StatusCode.INTERNAL)
            return isl_classifier_pb2.RecognitionResponse()
    
    def RecognizeHolistic(self, request, context):
        """
        Recognize ISL sign from MediaPipe Holistic landmarks.
        """
        try:
            # Preprocess holistic landmarks
            features = self._preprocess_holistic(list(request.landmarks))
            
            # Predict
            label, confidence, top_k, proc_time = self._predict(features)
            
            logger.debug(f"Recognized (holistic): {label} ({confidence:.2%})")
            
            return isl_classifier_pb2.RecognitionResponse(
                label=label,
                confidence=confidence,
                top_k=[
                    isl_classifier_pb2.Prediction(label=l, confidence=c)
                    for l, c in top_k
                ],
                processing_time_ms=proc_time,
                session_id=request.session_id,
                timestamp=request.timestamp
            )
            
        except Exception as e:
            logger.error(f"Error in RecognizeHolistic: {e}")
            context.set_details(str(e))
            context.set_code(grpc.StatusCode.INTERNAL)
            return isl_classifier_pb2.RecognitionResponse()
    
    def StreamRecognize(self, request_iterator, context):
        """
        Stream recognition for real-time processing.
        """
        for request in request_iterator:
            try:
                # Process each frame
                left_norm = self._normalize_hand(list(request.left_hand))
                right_norm = self._normalize_hand(list(request.right_hand))
                features = np.array(left_norm + right_norm, dtype=np.float32)
                
                label, confidence, top_k, proc_time = self._predict(features, top_k=3)
                
                yield isl_classifier_pb2.RecognitionResponse(
                    label=label,
                    confidence=confidence,
                    top_k=[
                        isl_classifier_pb2.Prediction(label=l, confidence=c)
                        for l, c in top_k
                    ],
                    processing_time_ms=proc_time,
                    session_id=request.session_id,
                    timestamp=request.timestamp
                )
                
            except Exception as e:
                logger.error(f"Error in StreamRecognize: {e}")
                yield isl_classifier_pb2.RecognitionResponse()
    
    def GetLabels(self, request, context):
        """
        Return all available ISL labels.
        """
        return isl_classifier_pb2.LabelsResponse(
            labels=self.labels,
            count=len(self.labels)
        )
    
    def HealthCheck(self, request, context):
        """
        Health check endpoint.
        """
        return isl_classifier_pb2.HealthResponse(
            healthy=True,
            model_version="1.0.0-signai",
            labels_count=len(self.labels),
            message="ISL Classifier Service is running"
        )


def serve(port: int = 50054, model_path: str = None):
    """
    Start the gRPC server.
    """
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=10),
        options=[
            ('grpc.max_receive_message_length', 50 * 1024 * 1024),
            ('grpc.max_send_message_length', 50 * 1024 * 1024),
        ]
    )
    
    service = ISLClassifierService(model_path=model_path)
    isl_classifier_pb2_grpc.add_ISLClassifierServicer_to_server(service, server)
    
    address = f'[::]:{port}'
    server.add_insecure_port(address)
    server.start()
    
    logger.info(f"=" * 50)
    logger.info(f"ISL Classifier gRPC Service")
    logger.info(f"Listening on port {port}")
    logger.info(f"Labels: {len(service.labels)} ISL signs")
    logger.info(f"=" * 50)
    
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        logger.info("Shutting down...")
        server.stop(0)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='ISL Classifier gRPC Service')
    parser.add_argument('--port', type=int, default=50054, help='gRPC port')
    parser.add_argument('--model', type=str, default=None, help='Path to ONNX model')
    
    args = parser.parse_args()
    serve(port=args.port, model_path=args.model)
