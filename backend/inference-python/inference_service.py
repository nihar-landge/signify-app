import grpc
from concurrent import futures
import onnxruntime as ort
import numpy as np
import json
import logging
import os
import sys

# Add proto directory to path
sys.path.append(os.path.join(os.path.dirname(__file__), "proto"))

import inference_pb2
import inference_pb2_grpc
from preprocessor import preprocess_landmarks, reconstruction_tensor

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

from signai_classifier import SignAIClassifier

class SignInferenceService(inference_pb2_grpc.SignInferenceServicer):
    def __init__(self, model_path, label_map_path):
        self.model_path = model_path
        self.label_map_path = label_map_path
        
        logger.info(f"Loading ONNX model from {model_path}...")
        self.session = ort.InferenceSession(model_path)
        
        logger.info(f"Loading label map from {label_map_path}...")
        with open(label_map_path) as f:
            self.sign_to_idx = json.load(f)
        self.idx_to_sign = {v: k for k, v in self.sign_to_idx.items()}
        
        # Load ISL Classifier
        logger.info("Loading ISL Classifier...")
        # Assuming models directory is same as model_path directory
        model_dir = os.path.dirname(model_path)
        self.isl_classifier = SignAIClassifier(model_dir)
        
        logger.info("Inference service initialized successfully (Dual Mode: ASL + ISL).")
    
    def Predict(self, request, context):
        try:
            # Check model type from session_id
            is_isl = request.session_id.endswith('|ISL')
            logger.info(f"PREDICT: session_id='{request.session_id}', model={'ISL' if is_isl else 'ASL'}")
            
            # Reconstruct tensor from request
            if request.tensor_content:
                landmarks = reconstruction_tensor(request.tensor_content, list(request.shape))
            else:
                # Fallback for old clients (should not happen with updated code)
                logger.warning("Received legacy landmarks list, expected tensor_content")
                # Assume [Time, 1629] or similar and try to reshape
                landmarks = np.array(request.landmarks).reshape(-1, 543, 3)

            # --- ISL Logic ---
            if is_isl:
                # ISL uses single frame (latest)
                # Input shape: [Batch, Time, 543, 3] usually
                # Take the last frame along time axis
                if landmarks.ndim >= 3:
                     # landmarks shape: [Batch, Time, 543, 3] or [Time, 543, 3]
                     # We want the last time step
                     # If batch dimension exists and > 1, we might need to loop. 
                     # But current Go client sends [30, 543, 3] (Window size 30).
                     # So let's take landmarks[-1] -> shape [543, 3]
                     
                     # Handle batch dim if present
                     if landmarks.ndim == 4:
                         last_frame = landmarks[0, -1] 
                     else:
                         last_frame = landmarks[-1]
                else:
                    # Should be [543, 3] already if just one frame sent
                    last_frame = landmarks

                label, confidence = self.isl_classifier.predict_from_holistic(last_frame)
                
                # Format response
                return inference_pb2.PredictResponse(top_k=[
                    inference_pb2.Prediction(
                        label=f"ISL: {label}",  # Prefix for UI distinguishing
                        confidence=confidence,
                        class_index=0 # Index not critical for ISL display
                    )
                ])

            # --- ASL Logic (Existing) ---
            
            # Preprocess
            input_tensor = preprocess_landmarks(landmarks)
            
            # Run inference
            try:
                # Check input name
                input_name = self.session.get_inputs()[0].name
                outputs = self.session.run(None, {input_name: input_tensor})
            except Exception as e:
                logger.error(f"ONNX Runtime Error: {e}")
                raise

            # Process outputs
            if len(outputs[0].shape) == 1:
                logits = outputs[0]
            else:
                # Batch 0
                logits = outputs[0][0]
            
            # Softmax (optional, but good for confidence)
            # exp_logits = np.exp(logits - np.max(logits))
            # probs = exp_logits / exp_logits.sum()
            
            # Get top-k (Top 5)
            # Using logits as confidence score proxy for now
            top_k_indices = np.argsort(logits)[-5:][::-1]
            
            # Map to labels
            predictions = []
            for idx in top_k_indices:
                confidence = float(logits[idx])
                label = self.idx_to_sign.get(idx, f"UNKNOWN_{idx}")
                
                predictions.append(inference_pb2.Prediction(
                    label=f"ASL: {label}", # Prefix for UI distinguishing
                    confidence=confidence,
                    class_index=int(idx)
                ))
            
            return inference_pb2.PredictResponse(top_k=predictions)
            
        except Exception as e:
            logger.error(f"Prediction failed: {e}", exc_info=True)
            context.set_details(str(e))
            context.set_code(grpc.StatusCode.INTERNAL)
            return inference_pb2.PredictResponse()

def serve():
    try:
        server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
        inference_pb2_grpc.add_SignInferenceServicer_to_server(
            SignInferenceService(
                model_path="models/signify_model.onnx", 
                label_map_path="models/sign_to_prediction_index_map.json"
            ), 
            server
        )
        server.add_insecure_port('[::]:50051')
        logger.info("Starting gRPC server on port 50051...")
        server.start()
        server.wait_for_termination()
    except Exception as e:
        logger.error(f"Server start failed: {e}")
        sys.exit(1)

if __name__ == '__main__':
    serve()
