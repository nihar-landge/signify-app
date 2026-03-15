# ISL Classifier Service
#
# Build: docker build -t signify/isl-classifier .
# Run: docker run -p 50054:50054 signify/isl-classifier

FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements
COPY requirements.txt .

# Install Python dependencies
RUN pip install --no-cache-dir -r requirements.txt

# Copy proto files and generate stubs
COPY isl_classifier.proto .
RUN python -m grpc_tools.protoc \
    -I. \
    --python_out=. \
    --grpc_python_out=. \
    isl_classifier.proto

# Copy application code
COPY server.py .
COPY models/ ./models/

# Expose gRPC port
EXPOSE 50054

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python -c "import grpc; ch = grpc.insecure_channel('localhost:50054'); grpc.channel_ready_future(ch).result(timeout=5)"

# Run the service
CMD ["python", "server.py", "--port", "50054"]
