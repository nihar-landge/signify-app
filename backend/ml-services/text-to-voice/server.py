import grpc
from concurrent import futures
import text_to_voice_pb2
import text_to_voice_pb2_grpc
from gtts import gTTS
import io
import logging
from pydub import AudioSegment

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class TextToVoiceService(text_to_voice_pb2_grpc.TextToVoiceServicer):
    def __init__(self):
        logger.info("Text-to-Voice Service initialized with gTTS")
    
    def Synthesize(self, request, context):
        print(f"DEBUG: Synthesize called for text: {request.text}")
        try:
            text = request.text.strip()
            
            if not text:
                logger.warning("Empty text received")
                return text_to_voice_pb2.SynthesisResponse(audio_data=b'')
            
            logger.info(f"Synthesizing text: {text[:50]}...")
            
            # Generate speech using gTTS
            tts = gTTS(text=text, lang='en', slow=False)
            
            # Save to bytes buffer (MP3)
            mp3_buffer = io.BytesIO()
            tts.write_to_fp(mp3_buffer)
            mp3_buffer.seek(0)
            
            # Convert MP3 to WAV for better compatibility
            audio = AudioSegment.from_mp3(mp3_buffer)
            
            # Export as WAV
            wav_buffer = io.BytesIO()
            audio.export(wav_buffer, format="wav")
            wav_buffer.seek(0)
            audio_data = wav_buffer.read()
            
            logger.info(f"Generated audio: {len(audio_data)} bytes")
            
            return text_to_voice_pb2.SynthesisResponse(audio_data=audio_data)
            
        except Exception as e:
            logger.error(f"Error in Synthesize: {e}")
            context.set_details(str(e))
            context.set_code(grpc.StatusCode.INTERNAL)
            return text_to_voice_pb2.SynthesisResponse(audio_data=b'')


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    text_to_voice_pb2_grpc.add_TextToVoiceServicer_to_server(
        TextToVoiceService(), server
    )
    server.add_insecure_port('[::]:50052')
    server.start()
    logger.info("Text-to-Voice service running on port 50052")
    server.wait_for_termination()


if __name__ == '__main__':
    serve()
