import numpy as np
import logging

logger = logging.getLogger(__name__)

def reconstruction_tensor(tensor_bytes: bytes, shape: list[int]) -> np.ndarray:
    """
    Reconstruct numpy array from raw bytes and shape.
    """
    try:
        # Convert bytes to float32 array
        arr = np.frombuffer(tensor_bytes, dtype=np.float32)
        
        # Reshape
        arr = arr.reshape(shape)
        return arr
    except Exception as e:
        logger.error(f"Failed to reconstruct tensor: {e}")
        raise

def preprocess_landmarks(landmarks: np.ndarray) -> np.ndarray:
    """
    Preprocess landmarks for model input.
    Input: [Batch, Time, 543, 3] or [Time, 543, 3]
    Output: [Batch, Time, 543, 3] tensor, dtype=float32
    """
    
    # Ensure float32
    landmarks = landmarks.astype(np.float32)
    
    # Handle NaN values (replace with 0)
    if np.isnan(landmarks).any():
        landmarks = np.nan_to_num(landmarks, nan=0.0)
    
    # Ensure correct shape [Time, 543, 3] (Rank 3)
    # The model expects [Time, Landmarks, Coords] where Time is dynamic
    
    # If we have [1, Time, 543, 3], squeeze the batch dim
    if len(landmarks.shape) == 4 and landmarks.shape[0] == 1:
        landmarks = landmarks.squeeze(0)
        
    return landmarks
