import grpc
from concurrent import futures
import sign_to_text_pb2
import sign_to_text_pb2_grpc
import cv2
import numpy as np
import mediapipe as mp
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Sign language mapping (simplified - expand for full ASL alphabet)
SIGN_GESTURES = {
    "thumbs_up": "Yes",
    "thumbs_down": "No",
    "open_palm": "Hello",
    "fist": "Stop",
    "peace": "Thank you",
    "ok_sign": "OK",
    "pointing": "You",
    "wave": "Goodbye",
}


class SignRecognitionService(sign_to_text_pb2_grpc.SignRecognitionServicer):
    def __init__(self):
        # Initialize MediaPipe Hands
        self.mp_hands = mp.solutions.hands
        self.hands = self.mp_hands.Hands(
            static_image_mode=True,
            max_num_hands=2,
            min_detection_confidence=0.7,
            min_tracking_confidence=0.5
        )
        self.mp_drawing = mp.solutions.drawing_utils
        logger.info("Sign Recognition Service initialized with MediaPipe")
    
    def RecognizeSign(self, request, context):
        try:
            # Decode image from bytes
            image_array = np.frombuffer(request.image_data, dtype=np.uint8)
            image = cv2.imdecode(image_array, cv2.IMREAD_COLOR)
            
            if image is None:
                logger.warning("Failed to decode image")
                return sign_to_text_pb2.RecognitionResponse(text="", confidence=0.0)
            
            # Convert BGR to RGB for MediaPipe
            image_rgb = cv2.cvtColor(image, cv2.COLOR_BGR2RGB)
            
            # Process with MediaPipe
            results = self.hands.process(image_rgb)
            
            if not results.multi_hand_landmarks:
                return sign_to_text_pb2.RecognitionResponse(text="", confidence=0.0)
            
            # Extract landmarks and classify gesture
            for hand_landmarks in results.multi_hand_landmarks:
                gesture, confidence = self._classify_gesture(hand_landmarks)
                
                if gesture:
                    text = SIGN_GESTURES.get(gesture, gesture)
                    logger.info(f"Recognized sign: {text} (confidence: {confidence:.2f})")
                    return sign_to_text_pb2.RecognitionResponse(
                        text=text,
                        confidence=confidence
                    )
            
            return sign_to_text_pb2.RecognitionResponse(text="", confidence=0.0)
            
        except Exception as e:
            logger.error(f"Error in RecognizeSign: {e}")
            context.set_details(str(e))
            context.set_code(grpc.StatusCode.INTERNAL)
            return sign_to_text_pb2.RecognitionResponse(text="", confidence=0.0)
    
    def _classify_gesture(self, hand_landmarks):
        """
        Classify hand gesture based on landmarks.
        This is a simplified implementation - in production, use a trained ML model.
        """
        landmarks = hand_landmarks.landmark
        
        # Get key landmarks
        thumb_tip = landmarks[self.mp_hands.HandLandmark.THUMB_TIP]
        index_tip = landmarks[self.mp_hands.HandLandmark.INDEX_FINGER_TIP]
        middle_tip = landmarks[self.mp_hands.HandLandmark.MIDDLE_FINGER_TIP]
        ring_tip = landmarks[self.mp_hands.HandLandmark.RING_FINGER_TIP]
        pinky_tip = landmarks[self.mp_hands.HandLandmark.PINKY_TIP]
        wrist = landmarks[self.mp_hands.HandLandmark.WRIST]
        
        index_mcp = landmarks[self.mp_hands.HandLandmark.INDEX_FINGER_MCP]
        middle_mcp = landmarks[self.mp_hands.HandLandmark.MIDDLE_FINGER_MCP]
        ring_mcp = landmarks[self.mp_hands.HandLandmark.RING_FINGER_MCP]
        pinky_mcp = landmarks[self.mp_hands.HandLandmark.PINKY_MCP]
        
        # Check if fingers are extended (tip above MCP in y)
        index_up = index_tip.y < index_mcp.y
        middle_up = middle_tip.y < middle_mcp.y
        ring_up = ring_tip.y < ring_mcp.y
        pinky_up = pinky_tip.y < pinky_mcp.y
        
        # Thumbs up: thumb up, all fingers down
        if thumb_tip.y < wrist.y - 0.15 and not (index_up or middle_up or ring_up or pinky_up):
            return "thumbs_up", 0.85
        
        # Open palm: all fingers extended
        if index_up and middle_up and ring_up and pinky_up:
            return "open_palm", 0.80
        
        # Peace sign: index and middle up, others down
        if index_up and middle_up and not ring_up and not pinky_up:
            return "peace", 0.82
        
        # Fist: no fingers extended
        if not (index_up or middle_up or ring_up or pinky_up):
            return "fist", 0.75
        
        # Pointing: only index extended
        if index_up and not (middle_up or ring_up or pinky_up):
            return "pointing", 0.78
        
        return None, 0.0


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    sign_to_text_pb2_grpc.add_SignRecognitionServicer_to_server(
        SignRecognitionService(), server
    )
    server.add_insecure_port('[::]:50051')
    server.start()
    logger.info("Sign-to-Text service running on port 50051")
    server.wait_for_termination()


if __name__ == '__main__':
    serve()
