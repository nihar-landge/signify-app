#!/usr/bin/env python3
"""
Create ONNX model for SignAI ISL classifier with trained weights.

This creates an ONNX model matching the SignAI TFLite model architecture
and loads the trained weights from the HDF5 file.

Architecture (from HDF5):
Input (84) → Dense(42, ReLU) → Dense(10, ReLU) → Dense(20, ReLU) → Dense(39, Softmax)
"""

import numpy as np
import onnx
from onnx import helper, TensorProto, numpy_helper
import csv
from pathlib import Path
import h5py


def create_signai_onnx_with_weights():
    """
    Create ONNX model with trained weights from HDF5.
    """
    
    # Paths
    base_dir = Path(__file__).parent.parent.parent / "signai_isl" / "SignAI" / "model" / "keypoint_classifier"
    h5_path = base_dir / "keypoint_classifier.hdf5"
    labels_path = base_dir / "keypoint_classifier_label.csv"
    
    # Load labels
    if labels_path.exists():
        with open(labels_path, encoding='utf-8-sig') as f:
            labels = [row[0] for row in csv.reader(f) if row]
        num_classes = len(labels)
        print(f"Found {num_classes} labels: {labels[:5]}...")
    else:
        num_classes = 39
        print(f"Labels not found, using default: {num_classes}")
    
    # Load weights from HDF5
    if not h5_path.exists():
        raise FileNotFoundError(f"HDF5 file not found: {h5_path}")
    
    with h5py.File(h5_path, 'r') as f:
        # Extract weights for each layer
        # Based on structure: dense_7(84→42), dense_8(42→10), dense_9(10→20), dense_10(20→39)
        
        W1 = f['model_weights/dense_7/dense_7/kernel:0'][:]  # (84, 42)
        B1 = f['model_weights/dense_7/dense_7/bias:0'][:]    # (42,)
        
        W2 = f['model_weights/dense_8/dense_8/kernel:0'][:]  # (42, 10)
        B2 = f['model_weights/dense_8/dense_8/bias:0'][:]    # (10,)
        
        W3 = f['model_weights/dense_9/dense_9/kernel:0'][:]  # (10, 20)
        B3 = f['model_weights/dense_9/dense_9/bias:0'][:]    # (20,)
        
        W4 = f['model_weights/dense_10/dense_10/kernel:0'][:] # (20, 39)
        B4 = f['model_weights/dense_10/dense_10/bias:0'][:]   # (39,)
    
    print(f"\nLoaded weights:")
    print(f"  Layer 1: W{W1.shape}, B{B1.shape}")
    print(f"  Layer 2: W{W2.shape}, B{B2.shape}")
    print(f"  Layer 3: W{W3.shape}, B{B3.shape}")
    print(f"  Layer 4: W{W4.shape}, B{B4.shape}")
    
    # Create ONNX tensors from weights
    W1_tensor = numpy_helper.from_array(W1.astype(np.float32), name='W1')
    B1_tensor = numpy_helper.from_array(B1.astype(np.float32), name='B1')
    W2_tensor = numpy_helper.from_array(W2.astype(np.float32), name='W2')
    B2_tensor = numpy_helper.from_array(B2.astype(np.float32), name='B2')
    W3_tensor = numpy_helper.from_array(W3.astype(np.float32), name='W3')
    B3_tensor = numpy_helper.from_array(B3.astype(np.float32), name='B3')
    W4_tensor = numpy_helper.from_array(W4.astype(np.float32), name='W4')
    B4_tensor = numpy_helper.from_array(B4.astype(np.float32), name='B4')
    
    # Create input/output
    X = helper.make_tensor_value_info('input', TensorProto.FLOAT, [1, 84])
    Y = helper.make_tensor_value_info('output', TensorProto.FLOAT, [1, num_classes])
    
    # Build network nodes
    nodes = [
        # Layer 1: Dense(84 → 42) + ReLU
        helper.make_node('MatMul', ['input', 'W1'], ['mm1']),
        helper.make_node('Add', ['mm1', 'B1'], ['add1']),
        helper.make_node('Relu', ['add1'], ['relu1']),
        
        # Layer 2: Dense(42 → 10) + ReLU
        helper.make_node('MatMul', ['relu1', 'W2'], ['mm2']),
        helper.make_node('Add', ['mm2', 'B2'], ['add2']),
        helper.make_node('Relu', ['add2'], ['relu2']),
        
        # Layer 3: Dense(10 → 20) + ReLU
        helper.make_node('MatMul', ['relu2', 'W3'], ['mm3']),
        helper.make_node('Add', ['mm3', 'B3'], ['add3']),
        helper.make_node('Relu', ['add3'], ['relu3']),
        
        # Layer 4: Dense(20 → 39) + Softmax
        helper.make_node('MatMul', ['relu3', 'W4'], ['mm4']),
        helper.make_node('Add', ['mm4', 'B4'], ['logits']),
        helper.make_node('Softmax', ['logits'], ['output'], axis=1),
    ]
    
    # Create graph
    graph = helper.make_graph(
        nodes,
        'signai_isl_classifier',
        [X],
        [Y],
        [W1_tensor, B1_tensor, W2_tensor, B2_tensor, 
         W3_tensor, B3_tensor, W4_tensor, B4_tensor]
    )
    
    # Create model
    model = helper.make_model(graph, opset_imports=[helper.make_opsetid('', 13)])
    model.ir_version = 7
    model.doc_string = "SignAI ISL Keypoint Classifier with trained weights"
    
    # Validate
    onnx.checker.check_model(model)
    print("\n✅ ONNX model created and validated!")
    
    return model, labels


def main():
    print("=" * 60)
    print("SignAI ISL ONNX Model Creator (with trained weights)")
    print("=" * 60)
    
    # Create model with trained weights
    model, labels = create_signai_onnx_with_weights()
    
    # Save model
    output_path = Path(__file__).parent / "models" / "signai_isl_classifier.onnx"
    output_path.parent.mkdir(parents=True, exist_ok=True)
    
    onnx.save(model, str(output_path))
    print(f"\n✅ ONNX model saved to: {output_path}")
    
    # Save labels as JSON for easy loading
    labels_path = Path(__file__).parent / "models" / "signai_isl_labels.json"
    import json
    with open(labels_path, 'w') as f:
        json.dump(labels, f, indent=2)
    print(f"✅ Labels saved to: {labels_path}")
    
    # Test with onnxruntime
    try:
        import onnxruntime as ort
        
        session = ort.InferenceSession(str(output_path))
        input_name = session.get_inputs()[0].name
        output_name = session.get_outputs()[0].name
        
        # Test inference with zeros
        test_input = np.zeros((1, 84), dtype=np.float32)
        result = session.run([output_name], {input_name: test_input})
        
        # Get top prediction
        probs = result[0][0]
        top_idx = np.argmax(probs)
        top_label = labels[top_idx] if top_idx < len(labels) else f"class_{top_idx}"
        
        print(f"\n✅ Test inference successful!")
        print(f"   Input shape: (1, 84)")
        print(f"   Output shape: {result[0].shape}")
        print(f"   Top prediction: '{top_label}' ({probs[top_idx]:.2%})")
        print(f"   Top 5 probabilities: {sorted(probs, reverse=True)[:5]}")
        
    except Exception as e:
        print(f"\n⚠️ ONNX Runtime test failed: {e}")
        import traceback
        traceback.print_exc()


if __name__ == "__main__":
    main()
