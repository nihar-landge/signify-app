# Signify — Developer Runbook

Everything you need to start, rebuild, and test the full Signify stack from scratch.

**Last updated:** 2026-03-16 (session 2)  
**Telephony provider:** Exotel (Twilio has been fully removed)

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

Verify the backend recognises Exotel as the telephony provider:

```bash
docker logs backend-go-backend-1 2>&1 | grep -i "exotel\|telephony\|provider"
# Expected:
# INFO: Exotel configured.
# Using Exotel as telephony provider
```

If `go-backend` stays in `(health: starting)` for more than 60 seconds, check all logs:

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

> **If the ngrok domain ever changes** (e.g. you switch to a different account), update it in **two places** and then rebuild the backend and the APK:
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

## 6. Exotel telephony setup

### Credentials (already in go-server/.env and docker-compose.yml)

| Variable | Value |
|---|---|
| `EXOTEL_SID` | `student1578` |
| `EXOTEL_KEY` | `a6ee35f5e7d2e5b655a2c317e10d19d7fe8be8248b0e25ea` |
| `EXOTEL_TOKEN` | `fdOe73ef7c71cf78f66b5f0423f84c5401fcf7d0feb35327` |
| `EXOTEL_FROM_NUMBER` | `09513886363` |

### Exotel Dashboard — one-time manual steps

These steps must be done by a human in the Exotel web dashboard. They do not need to be repeated unless the ExoPhone or domain changes.

1. **Create a Passthru app** in Exotel Dashboard:
   - Name: `Signify Incoming`
   - URL: `https://impertinently-uncommingled-aimee.ngrok-free.dev/exotel/incoming`
   - Method: `POST`

2. **Assign the ExoPhone `09513886363`** to the Passthru app above.

3. **Request Voicebot / WebSocket Media Streaming** for account `student1578`:
   - Email `support@exotel.in`
   - Subject: "Enable WebSocket Media Streaming (Voicebot) for account student1578"
   - This is an enterprise/premium feature. **Without it, real-time audio from Exotel calls cannot reach Deepgram** — real PSTN call STT will not work until Exotel confirms it is enabled.

### How the incoming call flow works (current state)

Until Exotel Media Streaming is enabled:

1. Caller dials `09513886363`.
2. Exotel hits `POST /exotel/incoming` with call metadata.
3. Backend returns ExoML: plays a brief "connecting" message, then starts recording.
4. App receives `incoming_call` over `/ws/app/global`.
5. User can accept → sign and TTS pipelines work; STT captions from the **phone call audio** do not work yet (no media stream).

Once Exotel Media Streaming is enabled:

- Swap the commented `<Connect><Stream>` block back into `HandleExotelIncoming` in `call_handler.go`.
- Rebuild the backend.
- Real-time Deepgram STT from the caller's voice will work.

### Pending after Media Streaming is enabled

```go
// In api/call_handler.go → HandleExotelIncoming
// Replace the current <Say>+<Record> ExoML with:
//
// xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
// <Response>
//   <Connect>
//     <Stream url="wss://impertinently-uncommingled-aimee.ngrok-free.dev/exotel/media" />
//   </Connect>
// </Response>`
```

After swapping in the above, rebuild:

```bash
cd ~/20-Projects/20-16-Final/backend
docker compose up --build go-backend -d
```

---

## 7. Test the three pipelines

### 7A — Test Mode (no real phone call needed)

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

### Pipeline 4: Real PSTN call (Exotel)

> **Prerequisite:** Exotel Dashboard steps in Section 6 must be complete.

1. Dial `09513886363` from any phone.
2. Check the Flutter app — a notification or `incoming_call` event should appear on the home screen.
3. Accept the call in the app.
4. Sign → captions flow (Pipelines 1 and 2) works immediately.
5. STT from the caller's voice requires Exotel Media Streaming to be enabled (see Section 6).

**Watch logs:**
```bash
docker logs -f backend-go-backend-1 2>&1 | grep -i "exotel\|incoming\|recording\|call"
```

---

## 8. Full startup checklist (daily)

```
[ ] Docker Desktop is running
[ ] cd ~/20-Projects/20-16-Final/backend && docker compose up -d
[ ] docker ps  →  go-backend shows (healthy)
[ ] docker logs backend-go-backend-1 2>&1 | grep -i "exotel"  →  "INFO: Exotel configured."
[ ] ngrok http 8080 --domain=impertinently-uncommingled-aimee.ngrok-free.dev  (in separate terminal)
[ ] curl https://impertinently-uncommingled-aimee.ngrok-free.dev/health  →  {"status":"ok"}
[ ] adb connect adb-e4947414-1F8z0K._adb-tls-connect._tcp
[ ] adb devices  →  device is listed
[ ] (if code changed) flutter analyze && flutter build apk --debug ...
[ ] (if code changed) adb push ... && adb shell pm install -r ...
[ ] adb shell am start -n com.signify.signify_app/.MainActivity
```

---

## 9. Rebuild after code changes

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

## 10. Useful log commands

```bash
# All backend logs (live):
docker logs -f backend-go-backend-1

# Filter by subsystem:
docker logs -f backend-go-backend-1 2>&1 | grep -i "stt\|deepgram"
docker logs -f backend-go-backend-1 2>&1 | grep -i "landmark\|sign\|stabiliz"
docker logs -f backend-go-backend-1 2>&1 | grep -i "tts\|elevenlabs"
docker logs -f backend-go-backend-1 2>&1 | grep -i "session\|ws\|websocket"
docker logs -f backend-go-backend-1 2>&1 | grep -i "exotel\|telephony\|provider"

# Flutter debug output from device:
adb logcat -s flutter

# All app logs from device (noisy):
adb logcat | grep -i signify
```

---

## 11. Troubleshooting

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

### Exotel call hangs up immediately / no incoming_call event in app
1. Confirm the Passthru app URL in Exotel Dashboard points to `https://impertinently-uncommingled-aimee.ngrok-free.dev/exotel/incoming`.
2. Confirm ngrok is running and the health check passes.
3. Check backend logs: `docker logs backend-go-backend-1 2>&1 | grep -i exotel`.
4. Confirm `docker logs backend-go-backend-1 2>&1 | grep -i "telephony\|provider"` shows `Using Exotel as telephony provider` (not Twilio).

### Exotel webhook hits backend but handler doesn't run (200 but no session created)
Exotel Passthru apps send **GET** requests (params in the query string), not POST.
The route is registered for both GET and POST. The handler reads `From` and `CallSid`
from query string (`c.Query()`) if not in form body. If you see `[GET] /exotel/incoming - 200`
in logs but no `Incoming Exotel call:` line, something changed — check `routes.go` that both
`app.Get` and `app.Post` are present for `/exotel/incoming`.

### Incoming call popup shows "Unknown" caller and signing does nothing after accepting
The `incoming_call` event from the backend uses `"message"` for the caller number and
`"text"` for the session ID (matching `AppEvent` JSON tags). The dialog in `app_router.dart`
must read `event['message']` and `event['text']` — **not** `event['from']` / `event['sessionId']`.
If the session ID is empty, `_startLandmarkPipeline()` aborts silently and the Sign button does nothing.
This was fixed in `app_router.dart:90-91`.

### Exotel PlaceCall API returns 400 / authentication error
- Verify `EXOTEL_SID`, `EXOTEL_KEY`, `EXOTEL_TOKEN` are correct in both `go-server/.env` and `docker-compose.yml`.
- Rebuild after any `.env` change: `docker compose up --build go-backend -d`.

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

## 12. Backend endpoint reference

| Endpoint | Direction | Purpose |
|---|---|---|
| `GET /health` | REST | Health check |
| `POST /api/sessions/start` | REST | Start real PSTN call via Exotel → returns `sessionId` |
| `POST /exotel/incoming` | REST ← Exotel | Exotel webhook for incoming calls; returns ExoML. Registered for **GET and POST** — Exotel Passthru apps send GET |
| `GET /exotel/incoming` | REST ← Exotel | Same handler as above (Exotel uses GET by default) |
| `POST /exotel/recording` | REST ← Exotel | Exotel webhook called when a recording is ready |
| `GET /exotel/recording` | REST ← Exotel | Same handler as above |
| `GET /ws/app/global` | WS ← backend | Receive `incoming_call` events |
| `GET /ws/app/events?sessionId=` | WS ← backend | Receive `sign_partial`, `sign_final`, `stt_partial`, `stt_final`, `call_status`, `error` |
| `GET /ws/app/landmarks?sessionId=&language=ASL\|ISL` | WS → backend | Flutter sends 1629-float landmark JSON packets |
| `GET /ws/app/audio?sessionId=&language=en-US\|hi\|mr` | WS → backend | Flutter sends raw PCM16 @ 16 kHz binary frames |

> **Future (when Exotel Media Streaming is enabled):**
> `GET /exotel/media` — WebSocket from Exotel carrying caller audio (linear16 @ 8000 Hz) → Deepgram STT

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

## 13. Key source files

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
| `api/call_handler.go` | All WS handlers; `HandleExotelIncoming`; `HandleExotelRecording`; `startSTTWorker`; landmark worker; TTS generation |
| `api/routes.go` | Route registration (includes `/exotel/incoming` and `/exotel/recording`) |
| `model/session.go` | `CallSession` struct; `ResetSTTState()`, `TryStartSTTWorker()` |
| `service/stt_service.go` | Deepgram WebSocket client; `StartSession(language)` |
| `service/landmark_processor.go` | ElevenLabs TTS call |
| `service/inference_client.go` | gRPC call to sign-model |
| `service/sign_stabilizer.go` | Window=5, min=3, cooldown=1.5s deduplication |
| `service/sentence_builder.go` | 1.2s silence gap → `sign_final` |
| `telephony/exotel_client.go` | Exotel REST API client; `PlaceCall`, `PlaceCallWithSession`; URL encoding fixed |
| `telephony/provider.go` | `TelephonyProvider` interface |
| `pkg/config/config.go` | Reads all env vars including `EXOTEL_SID`, `EXOTEL_KEY`, `EXOTEL_TOKEN`, `EXOTEL_FROM_NUMBER` |

### Config files (`~/20-Projects/20-16-Final/backend/`)

| File | Role |
|---|---|
| `docker-compose.yml` | All Exotel env vars passed to `go-backend` container |
| `go-server/.env` | Local dev credentials (Exotel replaces Twilio; never commit secrets) |

---

## 14. What's done vs. what's pending

### Done

- Twilio fully removed; Exotel credentials added to `.env` and `docker-compose.yml`
- `exotel_client.go`: Fixed `CallType=transcribe` → `CallType=trans`; fixed raw string form encoding → `url.Values` (correctly percent-encodes `+` in phone numbers and `?` in webhook URLs)
- `call_handler.go`: `HandleExotelIncoming` returns valid ExoML (`<Say>` + `<Record>`); `HandleExotelRecording` handler added
- `routes.go`: `/exotel/incoming` and `/exotel/recording` registered for **both GET and POST** (Exotel Passthru sends GET; params read from query string via `c.Query()`)
- `call_handler.go`: `HandleExotelIncoming` reads `From`/`CallSid` from `c.Query()` when not present in form body
- `app_router.dart`: Fixed incoming call dialog to read `event['message']` (caller number) and `event['text']` (session ID) instead of wrong keys `event['from']`/`event['sessionId']`
- `call_handler.go`: TTS now generated for **all** `sign_final` events (not just test sessions) — device speaker plays TTS during real PSTN calls
- `history_provider.dart`: Fixed type cast — backend returns `{"conversations": [...]}` map, not a bare array; unwraps `conversations` key with fallback
- Exotel Dashboard: Passthru app created and ExoPhone `09513886363` assigned
- Backend builds cleanly; logs confirm `INFO: Exotel configured.` and `Using Exotel as telephony provider`
- **End-to-end real PSTN call verified**: dial → popup → accept → sign → caption in app → TTS audio on device speaker ✓

### Pending (human action required)

1. **Exotel support**: Email `support@exotel.in` to enable WebSocket Media Streaming / Voicebot on account `student1578` — subject: "Enable WebSocket Media Streaming (Voicebot) for account student1578"
2. **After Media Streams enabled**: Swap `<Connect><Stream>` ExoML back into `HandleExotelIncoming` (commented block is already in `call_handler.go`), then rebuild

### Pending (code changes needed after Media Streams enabled)

- Activate the commented `<Connect><Stream>` block in `HandleExotelIncoming`
- Implement `/exotel/media` WebSocket handler that pipes Exotel audio (linear16 @ 8000 Hz) into `StartSTTWorker`
- End-to-end test: real PSTN call → Deepgram STT captions in the app
