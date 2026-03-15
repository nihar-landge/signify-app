FROM python:3.9-slim

WORKDIR /app

# Install system dependencies for audio processing
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python dependencies
COPY ml-services/text-to-voice/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy proto file and generate Python code
COPY proto/text_to_voice.proto /app/proto/
RUN python -m grpc_tools.protoc -I/app/proto --python_out=. --grpc_python_out=. /app/proto/text_to_voice.proto

# Copy application code
COPY ml-services/text-to-voice/server.py .

EXPOSE 50052

CMD ["python", "server.py"]
