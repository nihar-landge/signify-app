"""
Sign Language Inference Server with DUAL ASL + ISL Support

Supports both:
- ASL (American Sign Language) - 250 word-level signs (signify_model.onnx)
- ISL (Indian Sign Language) - 39 alphabet/number signs (signai_isl_classifier.onnx)

Returns predictions with prefixes:
- ASL:hello, ASL:thankyou, etc.
- ISL:a, ISL:1, etc.
"""
import grpc
from concurrent import futures
import sys
import os
import logging
import numpy as np
import json
from pathlib import Path

# Add the inference-python directory to path for classifiers
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../inference-python'))

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Import generated proto files
try:
    import inference_pb2
    import inference_pb2_grpc
    logger.info("Successfully imported proto files")
except ImportError as e:
    logger.error(f"Failed to import proto files: {e}")
    sys.exit(1)

# Import SignAI ISL classifier
try:
    from signai_classifier import SignAIClassifier
    ISL_AVAILABLE = True
    logger.info("SignAI ISL classifier available")
except ImportError as e:
    ISL_AVAILABLE = False
    logger.warning(f"SignAI ISL classifier not available: {e}")

# Try to load ASL ONNX model
ASL_AVAILABLE = False
try:
    import onnxruntime as ort
    ASL_AVAILABLE = True
    logger.info("ONNX Runtime available for ASL")
except ImportError:
    logger.warning("ONNX Runtime not available for ASL")


class SignInferenceService(inference_pb2_grpc.SignInferenceServicer):
    def __init__(self):
        logger.info("Sign Inference Service initializing...")
        self.request_count = 0
        
        # === ISL Model (SignAI - 39 alphabet/number signs) ===
        self.isl_classifier = None
        if ISL_AVAILABLE:
            try:
                self.isl_classifier = SignAIClassifier()
                logger.info(f"✅ ISL classifier loaded: {len(self.isl_classifier.labels)} signs")
            except Exception as e:
                logger.error(f"Failed to load ISL classifier: {e}")
        
        # === ASL Model (Signify - 250 word-level signs) ===
        self.asl_session = None
        self.asl_labels = []
        if ASL_AVAILABLE:
            try:
                models_dir = Path(__file__).parent.parent.parent / "inference-python" / "models"
                asl_model_path = models_dir / "signify_model.onnx"
                asl_labels_path = models_dir / "sign_to_prediction_index_map.json"
                
                if asl_model_path.exists():
                    self.asl_session = ort.InferenceSession(str(asl_model_path))
                    logger.info(f"✅ ASL model loaded from: {asl_model_path}")
                    
                    # Load labels (index -> word mapping)
                    if asl_labels_path.exists():
                        with open(asl_labels_path) as f:
                            label_map = json.load(f)
                        # Convert to index->label list
                        self.asl_labels = [""] * len(label_map)
                        for label, idx in label_map.items():
                            self.asl_labels[idx] = label
                        logger.info(f"✅ ASL labels loaded: {len(self.asl_labels)} signs")
                else:
                    logger.warning(f"ASL model not found: {asl_model_path}")
            except Exception as e:
                logger.error(f"Failed to load ASL model: {e}")
        
        # Log summary
        modes = []
        if self.isl_classifier:
            modes.append("ISL")
        if self.asl_session:
            modes.append("ASL")
        
        mode_str = " + ".join(modes) if modes else "MOCK"
        logger.info(f"Sign Inference Service initialized ({mode_str} Mode)")
    
    def _extract_hands_from_holistic(self, landmarks: np.ndarray) -> tuple:
        """Extract left and right hand landmarks from 543-point holistic data."""
        if len(landmarks) != 1629:
            return None, None
        
        data = landmarks.reshape(543, 3)
        lhand_start = 33 + 468  # 501
        rhand_start = lhand_start + 21  # 522
        
        left_hand = data[lhand_start:lhand_start + 21]
        right_hand = data[rhand_start:rhand_start + 21]
        
        left_valid = np.abs(left_hand).sum() > 0.01
        right_valid = np.abs(right_hand).sum() > 0.01
        
        return (left_hand if left_valid else None, 
                right_hand if right_valid else None)
    
    def _predict_asl(self, landmarks: np.ndarray) -> list:
        """Run ASL prediction on full holistic landmarks."""
        if not self.asl_session or len(self.asl_labels) == 0:
            return []
        
        try:
            # Get input details
            input_name = self.asl_session.get_inputs()[0].name
            input_shape = self.asl_session.get_inputs()[0].shape
            
            # Reshape input to match model expectations
            # The ASL model likely expects (batch, seq_len, features) or similar
            if len(landmarks.shape) == 1:
                landmarks = landmarks.reshape(1, -1)
            
            # Run inference
            result = self.asl_session.run(None, {input_name: landmarks.astype(np.float32)})
            probs = result[0][0] if len(result[0].shape) > 1 else result[0]
            
            # Get top-3 predictions
            top_indices = np.argsort(probs)[::-1][:3]
            
            predictions = []
            for idx in top_indices:
                if idx < len(self.asl_labels) and probs[idx] > 0.1:
                    predictions.append((self.asl_labels[idx], float(probs[idx])))
            
            return predictions
            
        except Exception as e:
            logger.debug(f"ASL prediction error: {e}")
            return []
    
    def _predict_isl(self, frame: np.ndarray) -> list:
        """Run ISL prediction on holistic landmarks."""
        if not self.isl_classifier:
            return []
        
        try:
            left_hand, right_hand = self._extract_hands_from_holistic(frame)
            
            if left_hand is None and right_hand is None:
                return []
            
            # Preprocess for SignAI (84-dim input)
            features = self.isl_classifier.preprocess_from_holistic(frame.reshape(543, 3))
            
            # Get top-k predictions
            topk = self.isl_classifier.predict_topk(features, k=3)
            return topk
            
        except Exception as e:
            logger.debug(f"ISL prediction error: {e}")
            return []
    
    def Predict(self, request, context):
        self.request_count += 1
        session_id = request.session_id
        
        # Parse input
        if request.tensor_content:
            landmarks = np.frombuffer(request.tensor_content, dtype=np.float32)
            if request.shape:
                try:
                    landmarks = landmarks.reshape(tuple(request.shape))
                except ValueError:
                    pass
        else:
            landmarks = np.array(list(request.landmarks), dtype=np.float32)
        
        logger.info(f"Request #{self.request_count} from {session_id} - {landmarks.shape if hasattr(landmarks, 'shape') else len(landmarks)}")
        
        # Get single frame for prediction
        if len(landmarks.shape) == 3:  # (N, 543, 3)
            frame = landmarks[-1].flatten()
        elif len(landmarks.shape) == 2:  # (543, 3)
            frame = landmarks.flatten()
        else:
            frame = landmarks
        
        if len(frame) != 1629:
            logger.warning(f"Expected 1629 floats, got {len(frame)}")
            context.set_details(f"Expected 1629 landmarks, got {len(frame)}")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            return inference_pb2.PredictResponse()
        
        predictions = []
        
        # === ISL Predictions ===
        isl_preds = self._predict_isl(frame)
        for label, conf in isl_preds:
            predictions.append(inference_pb2.Prediction(
                label=f"ISL:{label}",
                confidence=conf,
                class_index=0
            ))
        
        # === ASL Predictions ===
        asl_preds = self._predict_asl(landmarks)
        for label, conf in asl_preds:
            predictions.append(inference_pb2.Prediction(
                label=f"ASL:{label}",
                confidence=conf,
                class_index=1
            ))
        
        # Sort by confidence
        predictions.sort(key=lambda p: p.confidence, reverse=True)
        
        # Log top prediction
        if predictions:
            top = predictions[0]
            logger.info(f"Top prediction: {top.label} ({top.confidence:.2%})")
        
        return inference_pb2.PredictResponse(top_k=predictions[:6])  # Return top 6


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    inference_pb2_grpc.add_SignInferenceServicer_to_server(
        SignInferenceService(), server
    )
    server.add_insecure_port('[::]:50051')
    server.start()
    
    logger.info("=" * 60)
    logger.info("Sign Inference Service running on port 50051")
    logger.info("Supports: ASL (250 words) + ISL (39 signs)")
    logger.info("Ready to accept landmark data (1629 floats)")
    logger.info("=" * 60)
    
    server.wait_for_termination()


if __name__ == '__main__':
    try:
        serve()
    except KeyboardInterrupt:
        logger.info("Service stopped by user")
    except Exception as e:
        logger.error(f"Fatal error: {e}", exc_info=True)
        sys.exit(1)
