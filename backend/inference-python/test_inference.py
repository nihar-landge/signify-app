#!/usr/bin/env python3
import numpy as np
import onnxruntime as ort
import json
import os
import sys

# Add proto directory to path to simulate service environment
sys.path.append(os.path.join(os.path.dirname(__file__), "proto"))

def test_inference():
    print("Test 1: Check files...")
    model_path = "models/signify_model.onnx"
    map_path = "models/sign_to_prediction_index_map.json"
    
    if not os.path.exists(model_path):
        print(f"ERROR: Model not found at {model_path}")
        return False
    if not os.path.exists(map_path):
        print(f"ERROR: Map not found at {map_path}")
        return False
        
    print("Files found.")

    print("\nTest 2: Load Model...")
    try:
        session = ort.InferenceSession(model_path)
        print("Model loaded successfully.")
        
        # Print input details
        input_name = session.get_inputs()[0].name
        input_shape = session.get_inputs()[0].shape
        print(f"Input Name: {input_name}")
        print(f"Input Shape: {input_shape}")
    except Exception as e:
        print(f"ERROR loading model: {e}")
        return False

    print("\nTest 3: Run Inference (Dummy Data)...")
    try:
        # Create dummy input [1, 543, 3]
        dummy_input = np.random.randn(1, 543, 3).astype(np.float32)
        
        outputs = session.run(None, {input_name: dummy_input})
        raw_output = outputs[0]
        print(f"Raw output shape: {raw_output.shape}")
        
        if len(raw_output.shape) == 1:
            logits = raw_output
        else:
            logits = raw_output[0]
            
        print(f"Logits shape: {logits.shape}")
        
        # Load labels
        with open(map_path) as f:
            sign_to_idx = json.load(f)
        idx_to_sign = {v: k for k, v in sign_to_idx.items()}
        
        # Top-5
        top5 = np.argsort(logits)[-5:][::-1]
        print("Top 5 predictions (Random Input):")
        for idx in top5:
            label = idx_to_sign.get(str(idx), idx_to_sign.get(int(idx), "UNKNOWN"))
            print(f"  {label} ({idx}): {logits[idx]:.4f}")
            
    except Exception as e:
        print(f"ERROR running inference: {e}")
        return False
        
    print("\nSUCCESS: Inference service is ready to run.")
    return True

if __name__ == "__main__":
    if test_inference():
        sys.exit(0)
    else:
        sys.exit(1)
