#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

KEEP_STACK="${KEEP_STACK:-0}"
BACKEND_URL="${BACKEND_URL:-http://127.0.0.1:8080}"

cleanup() {
  if [[ "$KEEP_STACK" != "1" ]]; then
    docker compose down >/dev/null
  fi
}

cd "$BACKEND_DIR"
trap cleanup EXIT

echo "[1/5] Starting backend stack"
docker compose up --build -d postgres sign-model tts-model go-backend >/dev/null

echo "[2/5] Waiting for health endpoint"
for _ in {1..45}; do
  if curl -fsS "$BACKEND_URL/health" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

curl -fsS "$BACKEND_URL/health" >/dev/null

echo "[3/5] Verifying gRPC predict path"
docker compose exec -T sign-model python -c "import grpc, numpy as np; from proto import inference_pb2, inference_pb2_grpc; ch=grpc.insecure_channel('127.0.0.1:50051'); stub=inference_pb2_grpc.SignInferenceStub(ch); arr=np.random.randn(30,543,3).astype('float32'); resp=stub.Predict(inference_pb2.PredictRequest(session_id='smoke|ASL', tensor_content=arr.tobytes(), shape=[30,543,3])); assert len(resp.top_k) > 0; print('grpc predictions:', len(resp.top_k))"

echo "[4/5] Verifying landmarks websocket -> inference -> sign events"
BACKEND_URL="$BACKEND_URL" node "$SCRIPT_DIR/ws_smoke.mjs"

echo "[5/5] Pipeline smoke test passed"
