#!/usr/bin/env python3
"""
Test client for ISL Classifier gRPC Service.

Usage:
    python test_client.py
"""

import grpc
import numpy as np
import isl_classifier_pb2
import isl_classifier_pb2_grpc


def test_health_check(stub):
    """Test health check endpoint."""
    print("Testing HealthCheck...")
    response = stub.HealthCheck(isl_classifier_pb2.Empty())
    print(f"  Healthy: {response.healthy}")
    print(f"  Model: {response.model_version}")
    print(f"  Labels: {response.labels_count}")
    print(f"  Message: {response.message}")
    return response.healthy


def test_get_labels(stub):
    """Test get labels endpoint."""
    print("\nTesting GetLabels...")
    response = stub.GetLabels(isl_classifier_pb2.Empty())
    print(f"  Count: {response.count}")
    print(f"  Labels: {response.labels[:10]}...")
    return response.count > 0


def test_recognize_landmarks(stub):
    """Test landmark recognition with dummy data."""
    print("\nTesting RecognizeLandmarks...")
    
    # Create dummy hand landmarks (21 points × 2 coords = 42 floats)
    left_hand = [float(i * 0.01) for i in range(42)]
    right_hand = [float(i * 0.01) for i in range(42)]
    
    request = isl_classifier_pb2.LandmarksRequest(
        session_id="test-session",
        left_hand=left_hand,
        right_hand=right_hand,
        timestamp=1234567890
    )
    
    response = stub.RecognizeLandmarks(request)
    print(f"  Label: {response.label}")
    print(f"  Confidence: {response.confidence:.2%}")
    print(f"  Processing time: {response.processing_time_ms:.2f}ms")
    print(f"  Top-3:")
    for pred in response.top_k[:3]:
        print(f"    - {pred.label}: {pred.confidence:.2%}")
    
    return response.label != ""


def test_recognize_holistic(stub):
    """Test holistic recognition with dummy data."""
    print("\nTesting RecognizeHolistic...")
    
    # Create dummy holistic landmarks (543 × 3 = 1629 floats)
    landmarks = [float(i * 0.0001) for i in range(1629)]
    
    request = isl_classifier_pb2.HolisticRequest(
        session_id="test-session",
        landmarks=landmarks,
        timestamp=1234567890
    )
    
    response = stub.RecognizeHolistic(request)
    print(f"  Label: {response.label}")
    print(f"  Confidence: {response.confidence:.2%}")
    print(f"  Processing time: {response.processing_time_ms:.2f}ms")
    
    return response.label != ""


def main():
    print("=" * 50)
    print("ISL Classifier gRPC Client Test")
    print("=" * 50)
    
    # Connect to server
    channel = grpc.insecure_channel('localhost:50054')
    stub = isl_classifier_pb2_grpc.ISLClassifierStub(channel)
    
    results = []
    
    try:
        results.append(("HealthCheck", test_health_check(stub)))
        results.append(("GetLabels", test_get_labels(stub)))
        results.append(("RecognizeLandmarks", test_recognize_landmarks(stub)))
        results.append(("RecognizeHolistic", test_recognize_holistic(stub)))
        
        print("\n" + "=" * 50)
        print("Results:")
        for name, passed in results:
            status = "✅ PASS" if passed else "❌ FAIL"
            print(f"  {name}: {status}")
        
        all_passed = all(r[1] for r in results)
        print("=" * 50)
        print(f"Overall: {'✅ ALL TESTS PASSED' if all_passed else '❌ SOME TESTS FAILED'}")
        
    except grpc.RpcError as e:
        print(f"\n❌ Error connecting to server: {e}")
        print("Make sure the server is running: python server.py")


if __name__ == '__main__':
    main()
