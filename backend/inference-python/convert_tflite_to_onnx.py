#!/usr/bin/env python3
"""
Convert SignAI TFLite model to ONNX format.

This script converts the keypoint_classifier.tflite to ONNX format
for use with the existing ONNX-based inference service.

Usage:
    python convert_tflite_to_onnx.py

Requirements:
    pip install tf2onnx tensorflow onnx
"""

import os
import sys
import numpy as np
from pathlib import Path

def convert_hdf5_to_onnx():
    """Convert Keras HDF5 model to ONNX."""
    try:
        import tensorflow as tf
        import tf2onnx
        import onnx
    except ImportError as e:
        print(f"Missing dependency: {e}")
        print("Install with: pip install tensorflow tf2onnx onnx")
        return False
    
    # Paths
    base_dir = Path(__file__).parent.parent / "signai_isl" / "SignAI" / "model" / "keypoint_classifier"
    hdf5_path = base_dir / "keypoint_classifier.hdf5"
    onnx_output_path = Path(__file__).parent / "models" / "signai_isl_classifier.onnx"
    
    # Create output directory if needed
    onnx_output_path.parent.mkdir(parents=True, exist_ok=True)
    
    if not hdf5_path.exists():
        print(f"HDF5 model not found: {hdf5_path}")
        return False
    
    print(f"Loading Keras model from: {hdf5_path}")
    
    try:
        # Load the Keras model
        model = tf.keras.models.load_model(str(hdf5_path))
        print(f"Model loaded successfully")
        print(f"Input shape: {model.input_shape}")
        print(f"Output shape: {model.output_shape}")
        
        # Get input signature
        input_shape = model.input_shape
        if input_shape[0] is None:
            # Replace None batch dimension with 1
            concrete_input_shape = (1,) + input_shape[1:]
        else:
            concrete_input_shape = input_shape
        
        print(f"Concrete input shape: {concrete_input_shape}")
        
        # Convert to ONNX using tf2onnx
        input_signature = [tf.TensorSpec(concrete_input_shape, tf.float32, name='input')]
        
        # Convert the model
        onnx_model, _ = tf2onnx.convert.from_keras(model, input_signature=input_signature)
        
        # Save the ONNX model
        onnx.save(onnx_model, str(onnx_output_path))
        print(f"ONNX model saved to: {onnx_output_path}")
        
        # Verify the model
        onnx_model = onnx.load(str(onnx_output_path))
        onnx.checker.check_model(onnx_model)
        print("ONNX model verification passed!")
        
        # Test inference
        import onnxruntime as ort
        session = ort.InferenceSession(str(onnx_output_path))
        input_name = session.get_inputs()[0].name
        output_name = session.get_outputs()[0].name
        
        # Test with zeros
        test_input = np.zeros(concrete_input_shape, dtype=np.float32)
        result = session.run([output_name], {input_name: test_input})
        print(f"Test inference successful! Output shape: {result[0].shape}")
        
        return True
        
    except Exception as e:
        print(f"Error converting model: {e}")
        import traceback
        traceback.print_exc()
        return False


def convert_tflite_to_onnx_direct():
    """
    Alternative: Recreate the model architecture and convert.
    TFLite doesn't have direct ONNX conversion, so we recreate from HDF5.
    """
    # First try HDF5 conversion
    return convert_hdf5_to_onnx()


def create_equivalent_onnx_model():
    """
    Create an equivalent ONNX model from the TFLite model structure.
    This is a fallback if tf2onnx doesn't work.
    """
    try:
        import tensorflow as tf
        import onnx
        from onnx import helper, TensorProto, numpy_helper
    except ImportError as e:
        print(f"Missing dependency: {e}")
        return False
    
    # Load TFLite model to get weights
    base_dir = Path(__file__).parent.parent / "signai_isl" / "SignAI" / "model" / "keypoint_classifier"
    tflite_path = base_dir / "keypoint_classifier.tflite"
    
    if not tflite_path.exists():
        print(f"TFLite model not found: {tflite_path}")
        return False
    
    # Load TFLite interpreter
    interpreter = tf.lite.Interpreter(model_path=str(tflite_path))
    interpreter.allocate_tensors()
    
    input_details = interpreter.get_input_details()
    output_details = interpreter.get_output_details()
    
    print(f"TFLite Input: {input_details}")
    print(f"TFLite Output: {output_details}")
    
    # Get tensor details to extract weights
    tensor_details = interpreter.get_tensor_details()
    
    print(f"\nModel has {len(tensor_details)} tensors")
    for tensor in tensor_details:
        print(f"  {tensor['index']}: {tensor['name']} - {tensor['shape']} - {tensor['dtype']}")
    
    return True


if __name__ == "__main__":
    print("=" * 60)
    print("SignAI TFLite to ONNX Converter")
    print("=" * 60)
    
    success = convert_hdf5_to_onnx()
    
    if success:
        print("\n✅ Conversion successful!")
        print("\nONNX model saved to: backend/inference-python/models/signai_isl_classifier.onnx")
    else:
        print("\n❌ Conversion failed. Trying to analyze TFLite model...")
        create_equivalent_onnx_model()
