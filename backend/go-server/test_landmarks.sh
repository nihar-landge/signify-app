#!/bin/bash
# Test script for landmark WebSocket

SESSION_ID="${1:-test-session-123}"
HOST="${2:-localhost:8080}"

echo "Testing landmarks WebSocket with session: $SESSION_ID"
echo "Sending to: ws://$HOST/ws/app/landmarks?sessionId=$SESSION_ID"
echo ""

# 63 floats = 21 landmarks × 3 coordinates
LANDMARKS="0,0,0,0.02,0.03,0.01,0.04,0.06,0.02,0.06,0.09,0.03,0.08,0.12,0.04,0.10,0.15,0.05,0.12,0.18,0.06,0.14,0.21,0.07,0.16,0.24,0.08,0.18,0.27,0.09,0.20,0.30,0.10,0.22,0.33,0.11,0.24,0.36,0.12,0.26,0.39,0.13,0.28,0.42,0.14,0.30,0.45,0.15,0.32,0.48,0.16,0.34,0.51,0.17,0.36,0.54,0.18,0.38,0.57,0.19,0.40,0.60,0.20"

for i in $(seq 1 50); do
  JSON="{\"sessionId\":\"$SESSION_ID\",\"seq\":$i,\"ts_ms\":$(date +%s)000,\"present\":true,\"landmarks\":[$LANDMARKS],\"normalized\":true}"
  echo "$JSON"
  sleep 0.1
done
