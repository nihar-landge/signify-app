import onnxruntime as ort
import sys
import os

model_path = "models/signify_model.onnx"

if not os.path.exists(model_path):
    print(f"Error: Model file not found at {model_path}")
    sys.exit(1)

try:
    session = ort.InferenceSession(model_path)
    print("Model loaded successfully.")
    
    print("\n--- Input Details ---")
    for i, input_node in enumerate(session.get_inputs()):
        print(f"Input {i}:")
        print(f"  Name: {input_node.name}")
        print(f"  Shape: {input_node.shape}")
        print(f"  Type: {input_node.type}")
        
    print("\n--- Output Details ---")
    for i, output_node in enumerate(session.get_outputs()):
        print(f"Output {i}:")
        print(f"  Name: {output_node.name}")
        print(f"  Shape: {output_node.shape}")
        print(f"  Type: {output_node.type}")

except Exception as e:
    print(f"Error inspecting model: {e}")
