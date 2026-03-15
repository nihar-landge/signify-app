FROM python:3.9-slim

WORKDIR /app

# Install system dependencies for OpenCV (updated for newer Debian)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1 \
    libglib2.0-0 \
    libsm6 \
    libxrender1 \
    libxext6 \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python dependencies
COPY ml-services/sign-to-text/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy proto file and generate Python code
COPY proto/sign_to_text.proto /app/proto/
RUN python -m grpc_tools.protoc -I/app/proto --python_out=. --grpc_python_out=. /app/proto/sign_to_text.proto

# Copy application code
COPY ml-services/sign-to-text/server.py .

EXPOSE 50051

CMD ["python", "server.py"]
