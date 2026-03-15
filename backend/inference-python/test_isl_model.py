
import sys
import numpy as np
import logging
from signai_classifier import get_signai_classifier

logging.basicConfig(level=logging.INFO)

try:
    print("Loading ISL Classifier...")
    classifier = get_signai_classifier()
    print("Classifier loaded successfully.")
    
    print(f"Labels: len={len(classifier.labels)}")
    print(classifier.labels[:5])
    
    print("Running dummy prediction (Holistic 543x3)...")
    # Simulate holistic landmarks [543, 3]
    dummy_input = np.random.rand(543, 3).astype(np.float32)
    
    label, confidence = classifier.predict_from_holistic(dummy_input)
    print(f"Prediction result: {label} ({confidence:.2f})")
    
    print("Running dummy prediction (Features 84)...")
    dummy_features = np.random.rand(84).astype(np.float32)
    label, confidence = classifier.predict(dummy_features)
    print(f"Prediction result: {label} ({confidence:.2f})")
    
    print("TEST PASSED")

except Exception as e:
    print(f"TEST FAILED: {e}")
    import traceback
    traceback.print_exc()
