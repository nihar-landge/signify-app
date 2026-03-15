# Signify — Developer Runbook

Everything you need to start, rebuild, and test the full Signify stack from scratch.

---

## Directory layout

```
~/20-Projects/
├── 20-16-Final/backend/       ← Go backend + ML models (Docker)
└── UI/signify_app/            ← Flutter Android app
```

---

## Prerequisites

| Tool | Required version | Install |
|---|---|---|
| Docker Desktop | any recent | https://docker.com/products/docker-desktop |
| Flutter SDK | ≥ 3.32 | https://docs.flutter.dev/get-started/install |
| Android NDK | `27.0.12077973` | Android Studio → SDK Manager → NDK |
| Android platform-tools (adb) | any | bundled with Android Studio |
| ngrok | any | https://ngrok.com/download |

> **NDK**: the exact version matters for `hand_landmarker`. If the build fails with an NDK error, open `signify_app/android/app/build.gradle` and confirm `ndkVersion "27.0.12077973"` is set.

---

## 1. Start the backend

```bash
cd ~/20-Projects/20-16-Final/backend

# First time or after any Go/Python source change:
docker compose up --build -d

# Subsequent starts (no code changes):
docker compose up -d
```

Wait ~30 seconds for the healthcheck to pass:

```bash
docker ps
# Expected:
# backend-go-backend-1   Up X seconds (healthy)   0.0.0.0:8080->8080/tcp
# backend-tts-model-1    Up X seconds
# backend-sign-model-1   Up X seconds
# backend-postgres-1     Up X seconds (healthy)
```

If `go-backend` stays in `(health: starting)` for more than 60 seconds, check logs:

```bash
docker logs backend-go-backend-1
```

---

## 2. Verify the ngrok tunnel is active

The backend's public URL is a **static** ngrok subdomain baked into `docker-compose.yml`:

```
https://impertinently-uncommingled-aimee.ngrok-free.dev  →  localhost:8080
```

Start ngrok **once** in a dedicated terminal and leave it running:

```bash
ngrok http 8080 --domain=impertinently-uncommingled-aimee.ngrok-free.dev
```

Confirm the tunnel is alive:

```bash
curl -s https://impertinently-uncommingled-aimee.ngrok-free.dev/health
# Expected: {"service":"signify-backend","status":"ok"}
```

> **If the ngrok domain ever changes** (e.g. you switch to a different account), you must update it in **two places** and then rebuild the backend and the APK:
> 1. `docker-compose.yml` — `PUBLIC_BASE_URL` and `PUBLIC_WS_URL` env vars
> 2. The `flutter build apk` command — `--dart-define=SIGNIFY_NGROK_HOST=<new-host>`

---

## 3. Connect to the Android device over wireless ADB

The device is a **Realme RMX3521 (Android 14)** paired via Wi-Fi.

```bash
# One-time pair (only needed after factory reset or first time):
adb pair <device-ip>:<pair-port>   # shown in Developer Options → Wireless debugging

# Connect every session:
adb connect adb-e4947414-1F8z0K._adb-tls-connect._tcp

# Verify:
adb devices
# Expected:
# adb-e4947414-1F8z0K._adb-tls-connect._tcp   device
```

If the mDNS name doesn't resolve (you get "nodename not known"):

```bash
# Find the device IP from Developer Options → Wireless debugging, then:
adb connect <device-ip>:<port>
```

> **Never use `adb install` for this device** — it times out over wireless. Always use the two-step push+install method shown in Section 4.

---

## 4. Build and install the Flutter app

```bash
cd ~/20-Projects/UI/signify_app

# Lint check first — must pass with 0 issues:
flutter analyze

# Build debug APK (always use these exact --dart-define flags):
flutter build apk --debug \
  --dart-define=SIGNIFY_USE_NGROK=true \
  --dart-define=SIGNIFY_NGROK_HOST=impertinently-uncommingled-aimee.ngrok-free.dev

# Push to device (push is reliable; direct install times out over wireless):
adb push build/app/outputs/flutter-apk/app-debug.apk /data/local/tmp/signify.apk

# Install:
adb shell pm install -r /data/local/tmp/signify.apk

# Launch:
adb shell am start -n com.signify.signify_app/.MainActivity
```

Expected output:
```
✓ Built build/app/outputs/flutter-apk/app-debug.apk
app-debug.apk: 1 file pushed. X MB/s ...
Success
Starting: Intent { cmp=com.signify.signify_app/.MainActivity }
```

---

## 5. Configure the app (first run only)

1. On first launch: select **ASL** or **ISL** on the Welcome screen.
2. Open **Settings** tab:
   - **Sign Language**: ASL (American) or ISL (Indian)
   - **Caller's Spoken Language**: English / Hindi / Marathi — pick whatever you'll speak during the STT test
   - **Voice Output**: ON (plays TTS audio through speaker)
   - Backend config is set at compile time via `--dart-define`; the Settings fields for ngrok are display-only.

---

## 6. Test the three pipelines

### 6A — Test Mode (no real phone call needed)

Tap **Test Mode** on the home screen. This creates a `test-session-<uuid>` that the backend auto-accepts. All three pipelines run through this single session.

---

### Pipeline 1: Sign → Text (sign language recognition)

1. Enter Test Mode.
2. Grant camera permission when prompted.
3. **Hold the Sign button** (hand icon) while signing.
4. Release when done.

**Expected result:**
- While holding: landmark packets stream to `/ws/app/landmarks`
- After a stable sign is detected: a caption bubble appears in the upper area
- `sign_final` events include base64 TTS audio → triggers Pipeline 2

**Watch logs:**
```bash
docker logs -f backend-go-backend-1 2>&1 | grep -i "sign\|landmark\|stabiliz\|sentence"
```

Look for:
```
STABILIZED SIGN: HELLO for session test-session-...
sign_final event sent: text=HELLO
```

---

### Pipeline 2: TTS audio playback

Triggered automatically by `sign_final` events when **Voice Output** is ON.

1. After a sign is recognised (Pipeline 1), the backend calls ElevenLabs and returns base64 audio in the `sign_final` event.
2. The app decodes and plays it through the device speaker.

**No extra steps needed.** If you hear nothing:
- Check Settings → Voice Output is ON.
- Check backend logs for TTS errors:

```bash
docker logs -f backend-go-backend-1 2>&1 | grep -i "tts\|elevenlabs\|audio"
```

---

### Pipeline 3: Voice → STT (speech-to-text captions)

1. Enter Test Mode.
2. Grant microphone permission when prompted.
3. **Hold the Mic button** (microphone icon) while speaking.
4. Release when done.

**Expected result:**
- While holding: raw PCM16 @ 16 kHz streams to `/ws/app/audio?sessionId=...&language=en-US`
- Deepgram returns partial transcriptions → `stt_partial` caption bubbles appear (dimmed)
- When you stop speaking: `stt_final` fires → bold caption bubble appears

**Watch logs:**
```bash
docker logs -f backend-go-backend-1 2>&1 | grep -i "stt\|deepgram\|audio\|lang"
```

Look for:
```
App audio WS connected: sessionId=test-session-..., (linear16@16kHz, lang=en-US)
Starting STT worker for session test-session-... (trigger=app_mic)
STT session started: sessionId=test-session-...
STT final for session test-session-...: hello this is a test
```

---

## 7. Full startup checklist (daily)

```
[ ] Docker Desktop is running
[ ] cd ~/20-Projects/20-16-Final/backend && docker compose up -d
[ ] docker ps  →  go-backend shows (healthy)
[ ] ngrok http 8080 --domain=impertinently-uncommingled-aimee.ngrok-free.dev  (in separate terminal)
[ ] curl https://impertinently-uncommingled-aimee.ngrok-free.dev/health  →  {"status":"ok"}
[ ] adb connect adb-e4947414-1F8z0K._adb-tls-connect._tcp
[ ] adb devices  →  device is listed
[ ] (if code changed) flutter analyze && flutter build apk --debug ...
[ ] (if code changed) adb push ... && adb shell pm install -r ...
[ ] adb shell am start -n com.signify.signify_app/.MainActivity
```

---

## 8. Rebuild after code changes

### Backend only (Go source changed)

```bash
cd ~/20-Projects/20-16-Final/backend
docker compose up --build go-backend -d
```

Wait for `(healthy)` before testing.

### Flutter only (Dart source changed)

```bash
cd ~/20-Projects/UI/signify_app
flutter analyze                             # must be 0 issues
flutter build apk --debug \
  --dart-define=SIGNIFY_USE_NGROK=true \
  --dart-define=SIGNIFY_NGROK_HOST=impertinently-uncommingled-aimee.ngrok-free.dev
adb push build/app/outputs/flutter-apk/app-debug.apk /data/local/tmp/signify.apk
adb shell pm install -r /data/local/tmp/signify.apk
adb shell am start -n com.signify.signify_app/.MainActivity
```

### Both changed

Run the backend rebuild first, then the Flutter rebuild.

---

## 9. Useful log commands

```bash
# All backend logs (live):
docker logs -f backend-go-backend-1

# Filter by subsystem:
docker logs -f backend-go-backend-1 2>&1 | grep -i "stt\|deepgram"
docker logs -f backend-go-backend-1 2>&1 | grep -i "landmark\|sign\|stabiliz"
docker logs -f backend-go-backend-1 2>&1 | grep -i "tts\|elevenlabs"
docker logs -f backend-go-backend-1 2>&1 | grep -i "session\|ws\|websocket"

# Flutter debug output from device:
adb logcat -s flutter

# All app logs from device (noisy):
adb logcat | grep -i signify
```

---

## 10. Troubleshooting

### Camera shows infinite spinner in Test Mode
The camera permission race was fixed. If it regresses, check `call_screen.dart` — `_callAvailableCamerasProvider` listener must be present at the top of `_InCallViewState.build()`.

### WebSocket connects but signs are never recognized
- Confirm landmarks are 1629 floats (`present=true` frames).
- Backend silently drops frames where both wrists are below y > 0.7 (hands-down idle detection).
- Try signing with both hands clearly raised in front of the camera.

### STT never produces captions (mic button held but nothing appears)
Check backend logs for `STT worker starting`. If absent:
1. Confirm `DEEPGRAM_API_KEY` is in `docker-compose.yml` (not just `.env`).
2. Confirm `docker compose up --build go-backend -d` was run after the last change to `docker-compose.yml`.
3. Confirm the `language` param is sent in the WS URL — check Flutter debug logs: `MicStreamClient: connecting wss://...audio?sessionId=...&language=en-US`.

### STT starts but transcription is wrong / empty
- Verify the **Caller's Spoken Language** setting matches what you are speaking.
- Deepgram `nova-2` supports `en-US`, `hi`, `mr`. Do not use unsupported codes.

### `adb connect` fails with "nodename not known"
The mDNS name doesn't always resolve on all networks. Find the device IP in Settings → Developer Options → Wireless debugging, then:
```bash
adb connect <device-ip>:<port>
```

### Backend container exits immediately after `docker compose up`
```bash
docker logs backend-go-backend-1
```
Common causes: missing env var, Postgres not ready yet (re-run `docker compose up -d` — Postgres may need a moment on first boot).

### `flutter build` fails with NDK error
In Android Studio → SDK Manager → NDK (Side by side), install exactly `27.0.12077973`. Confirm in `signify_app/android/app/build.gradle`:
```gradle
android {
    ndkVersion "27.0.12077973"
    ...
}
```

---

## 11. Backend endpoint reference

| Endpoint | Direction | Purpose |
|---|---|---|
| `GET /health` | REST | Health check |
| `POST /api/sessions/start` | REST | Start real PSTN call → returns `sessionId` |
| `GET /ws/app/global` | WS ← backend | Receive `incoming_call` events |
| `GET /ws/app/events?sessionId=` | WS ← backend | Receive `sign_partial`, `sign_final`, `stt_partial`, `stt_final`, `call_status`, `error` |
| `GET /ws/app/landmarks?sessionId=&language=ASL\|ISL` | WS → backend | Flutter sends 1629-float landmark JSON packets |
| `GET /ws/app/audio?sessionId=&language=en-US\|hi\|mr` | WS → backend | Flutter sends raw PCM16 @ 16 kHz binary frames |

### Landmark packet format (1629 floats, JSON)

```
[0   .. 98 ]  Pose       33 pts × 3 (x, y, z)
[99  ..1502]  Face      468 pts × 3 (zero-filled — not used)
[1503..1565]  Left hand  21 pts × 3
[1566..1628]  Right hand 21 pts × 3
```

### Key event shapes (received on `/ws/app/events`)

```json
{ "type": "sign_partial", "text": "HELLO",  "timestamp": 1234567890 }
{ "type": "sign_final",   "text": "HELLO",  "audio": "<base64-mp3>", "timestamp": 1234567890 }
{ "type": "stt_partial",  "text": "hello",  "timestamp": 1234567890 }
{ "type": "stt_final",    "text": "hello.", "timestamp": 1234567890 }
{ "type": "call_status",  "status": "created|dialing|connected|completed", "timestamp": 1234567890 }
{ "type": "error",        "message": "...", "timestamp": 1234567890 }
```

---

## 12. Key source files

### Flutter (`~/20-Projects/UI/signify_app/lib/`)

| File | Role |
|---|---|
| `core/services/websocket_service.dart` | `SignifyConfig` — compile-time URL constants |
| `core/services/landmark_websocket_client.dart` | Connects to `/ws/app/landmarks`, sends landmark packets, receives sign events |
| `core/services/mic_stream_client.dart` | Connects to `/ws/app/audio`, streams PCM16 mic audio |
| `core/services/call_service.dart` | `CallSessionService` — manages both WS connections; exposes `captionStream` and `audioStream` |
| `core/providers/providers.dart` | All Riverpod providers; `AppSettings` (incl. `sttLanguage`); `CallNotifier` |
| `features/call/call_screen.dart` | In-call UI; Sign button, Mic button, caption bubbles, camera preview |
| `features/settings/settings_screen.dart` | Settings UI including STT language selector |

### Backend (`~/20-Projects/20-16-Final/backend/go-server/internal/`)

| File | Role |
|---|---|
| `api/call_handler.go` | All WS handlers; `startSTTWorker`; landmark worker; TTS generation |
| `api/routes.go` | Route registration |
| `model/session.go` | `CallSession` struct; `ResetSTTState()`, `TryStartSTTWorker()` |
| `service/stt_service.go` | Deepgram WebSocket client; `StartSession(language)` |
| `service/landmark_processor.go` | ElevenLabs TTS call |
| `service/inference_client.go` | gRPC call to sign-model |
| `service/sign_stabilizer.go` | Window=5, min=3, cooldown=1.5s deduplication |
| `service/sentence_builder.go` | 1.2s silence gap → `sign_final` |
