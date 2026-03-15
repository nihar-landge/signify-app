import numpy as np
import grpc
import sys
import os
import time

# Add proto directory to path
sys.path.append(os.path.join(os.path.dirname(__file__), "proto"))

import inference_pb2
import inference_pb2_grpc

def run_test():
    print("Testing Sign Inference Service...")
    
    # Create dummy data: 30 frames, 543 landmarks, 3 coords
    # Random values simulating normalized landmarks
    frames = 30
    landmarks = 543
    coords = 3
    dummy_data = np.random.rand(frames, landmarks, coords).astype(np.float32)
    
    print(f"Generated dummy data shape: {dummy_data.shape}")
    
    tensor_bytes = dummy_data.tobytes()
    shape = [frames, landmarks, coords] # Rank 3 input
    
    # Connect to server
    try:
        channel = grpc.insecure_channel('localhost:50051')
        stub = inference_pb2_grpc.SignInferenceStub(channel)
        
        request = inference_pb2.PredictRequest(
            session_id="test-session-cli",
            tensor_content=tensor_bytes,
            shape=shape
        )
        
        print("Sending request...")
        start_time = time.time()
        response = stub.Predict(request)
        end_time = time.time()
        
        print(f"\nResponse received in {(end_time - start_time)*1000:.2f} ms:")
        for i, pred in enumerate(response.top_k):
            print(f"{i+1}. {pred.label} (Confidence: {pred.confidence:.4f})")
            
    except grpc.RpcError as e:
        print(f"RPC Error: {e.code()} - {e.details()}")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == '__main__':
    run_test()
