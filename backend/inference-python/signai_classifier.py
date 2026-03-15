"""
SignAI ISL Classifier - ONNX-based inference

This classifier uses the converted ONNX model from SignAI for ISL recognition.
Input: 84 floats (21 landmarks × 2 coords × 2 hands)
Output: ISL character/symbol prediction with confidence
"""

import numpy as np
import json
from pathlib import Path
import logging

import onnxruntime as ort

logger = logging.getLogger(__name__)


class SignAIClassifier:
    """
    ISL Keypoint Classifier using ONNX model.
    
    Expected input: 84 floats (normalized hand landmarks)
    - Two hands × 21 landmarks × 2 coords (x, y)
    - Wrist-normalized and max-value normalized
    
    Labels: ISL alphabets, numerals, and special symbols
    """
    
    def __init__(self, model_dir: str = None):
        """
        Initialize the SignAI ONNX classifier.
        
        Args:
            model_dir: Path to directory containing ONNX model and labels.
                      Defaults to inference-python/models/
        """
        if model_dir is None:
            model_dir = Path(__file__).parent / "models"
        
        model_dir = Path(model_dir)
        
        # Load ONNX model
        model_path = model_dir / "signai_isl_classifier.onnx"
        if not model_path.exists():
            raise FileNotFoundError(
                f"ONNX model not found: {model_path}. "
                "Run 'python create_signai_onnx.py' to generate it."
            )
        
        # Create inference session
        self.session = ort.InferenceSession(str(model_path))
        self.input_name = self.session.get_inputs()[0].name
        self.output_name = self.session.get_outputs()[0].name
        
        logger.info(f"Loaded SignAI ONNX model from: {model_path}")
        
        # Load labels
        labels_path = model_dir / "signai_isl_labels.json"
        if labels_path.exists():
            with open(labels_path, 'r') as f:
                self.labels = json.load(f)
        else:
            # Fallback: try CSV
            csv_path = model_dir / "signai_isl_labels.csv"
            if csv_path.exists():
                import csv
                with open(csv_path, encoding='utf-8-sig') as f:
                    self.labels = [row[0] for row in csv.reader(f) if row]
            else:
                self.labels = [f"class_{i}" for i in range(39)]
        
        logger.info(f"Loaded {len(self.labels)} ISL labels")
    
    def preprocess_landmarks(self, 
                            left_hand: list = None, 
                            right_hand: list = None) -> np.ndarray:
        """
        Preprocess hand landmarks to 84-dimensional input.
        
        Args:
            left_hand: List of 21 (x, y) tuples or None
            right_hand: List of 21 (x, y) tuples or None
        
        Returns:
            84-dimensional normalized feature vector
        """
        # Initialize with zeros (42 for each hand)
        left_features = [0.0] * 42
        right_features = [0.0] * 42
        
        if left_hand and len(left_hand) >= 21:
            left_features = self._normalize_hand(left_hand)
        
        if right_hand and len(right_hand) >= 21:
            right_features = self._normalize_hand(right_hand)
        
        # Combine: left (42) + right (42) = 84
        features = left_features + right_features
        return np.array(features, dtype=np.float32)
    
    def _normalize_hand(self, landmarks: list) -> list:
        """
        Normalize hand landmarks (wrist-relative, max-normalized).
        
        Args:
            landmarks: List of 21 (x, y) or (x, y, z) tuples
        
        Returns:
            42-dimensional normalized vector (21 × 2)
        """
        # Extract x, y only
        points = []
        for lm in landmarks[:21]:
            if isinstance(lm, (list, tuple)):
                x, y = lm[0], lm[1]
            elif hasattr(lm, 'x'):
                x, y = lm.x, lm.y
            else:
                x, y = 0, 0
            points.append([x, y])
        
        # Normalize relative to wrist (landmark 0)
        base_x, base_y = points[0]
        normalized = [[x - base_x, y - base_y] for x, y in points]
        
        # Flatten to 42 values
        flat = [coord for point in normalized for coord in point]
        
        # Max-normalize
        max_val = max(abs(v) for v in flat) if flat else 1.0
        if max_val > 0:
            flat = [v / max_val for v in flat]
        
        return flat
    
    def preprocess_from_holistic(self, holistic_landmarks: np.ndarray) -> np.ndarray:
        """
        Convert 543-landmark holistic input to 84-dimensional SignAI input.
        
        Args:
            holistic_landmarks: Shape (543, 3) from MediaPipe Holistic
                Order: Pose (33) + Face (468) + LeftHand (21) + RightHand (21)
        
        Returns:
            84-dimensional feature vector for SignAI model
        """
        # MediaPipe Holistic landmark indices
        lhand_start = 33 + 468  # 501
        rhand_start = lhand_start + 21  # 522
        
        left_hand = holistic_landmarks[lhand_start:lhand_start + 21]
        right_hand = holistic_landmarks[rhand_start:rhand_start + 21]
        
        # Check if hands have valid data
        left_valid = np.abs(left_hand).sum() > 0.01
        right_valid = np.abs(right_hand).sum() > 0.01
        
        left_list = [(p[0], p[1]) for p in left_hand] if left_valid else None
        right_list = [(p[0], p[1]) for p in right_hand] if right_valid else None
        
        return self.preprocess_landmarks(left_list, right_list)
    
    def predict(self, features: np.ndarray) -> tuple:
        """
        Run prediction on preprocessed features.
        
        Args:
            features: 84-dimensional normalized feature vector
        
        Returns:
            (label, confidence) tuple
        """
        # Ensure correct shape
        if features.ndim == 1:
            features = features.reshape(1, -1)
        
        # Run inference
        result = self.session.run(
            [self.output_name],
            {self.input_name: features.astype(np.float32)}
        )
        probs = result[0][0]
        
        # Get top prediction
        top_idx = int(np.argmax(probs))
        confidence = float(probs[top_idx])
        label = self.labels[top_idx] if top_idx < len(self.labels) else f"class_{top_idx}"
        
        return label, confidence
    
    def predict_topk(self, features: np.ndarray, k: int = 5) -> list:
        """
        Get top-k predictions.
        
        Args:
            features: 84-dimensional normalized feature vector
            k: Number of top predictions to return
        
        Returns:
            List of (label, confidence) tuples
        """
        if features.ndim == 1:
            features = features.reshape(1, -1)
        
        result = self.session.run(
            [self.output_name],
            {self.input_name: features.astype(np.float32)}
        )
        probs = result[0][0]
        
        top_indices = np.argsort(probs)[::-1][:k]
        
        return [
            (self.labels[idx] if idx < len(self.labels) else f"class_{idx}", 
             float(probs[idx]))
            for idx in top_indices
        ]
    
    def predict_from_holistic(self, holistic_landmarks: np.ndarray) -> tuple:
        """
        Full pipeline: holistic landmarks → prediction.
        
        Args:
            holistic_landmarks: Shape (543, 3) from MediaPipe Holistic
        
        Returns:
            (label, confidence) tuple
        """
        features = self.preprocess_from_holistic(holistic_landmarks)
        return self.predict(features)


# Singleton instance
_classifier = None


def get_signai_classifier(model_dir: str = None) -> SignAIClassifier:
    """Get or create SignAI classifier instance."""
    global _classifier
    if _classifier is None:
        _classifier = SignAIClassifier(model_dir)
    return _classifier


if __name__ == "__main__":
    # Test the classifier
    logging.basicConfig(level=logging.INFO)
    
    try:
        classifier = get_signai_classifier()
        print(f"Loaded SignAI ONNX classifier with {len(classifier.labels)} labels")
        print(f"Labels: {classifier.labels}")
        
        # Test with dummy input
        dummy_input = np.zeros(84, dtype=np.float32)
        label, conf = classifier.predict(dummy_input)
        print(f"\nTest prediction (zeros): {label} ({conf:.2%})")
        
        # Test top-k
        topk = classifier.predict_topk(dummy_input, k=5)
        print("\nTop-5 predictions:")
        for lbl, c in topk:
            print(f"  {lbl}: {c:.2%}")
        
    except Exception as e:
        print(f"Error: {e}")
        import traceback
        traceback.print_exc()
