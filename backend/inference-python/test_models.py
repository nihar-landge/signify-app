#!/usr/bin/env python3
"""
Signify Model Test Mode — Interactive ASL/ISL Sign Recognition

Tests ONNX models using your webcam + MediaPipe to verify the
sign-to-text pipeline works end-to-end.

Usage:  python test_models.py
Keys:   q=quit  s=switch ASL/ISL  c=clear  space=freeze
"""

import sys, os, json, time, collections
import numpy as np

# ── Dependency check ──
def _check():
    m = []
    for pkg in ["cv2", "mediapipe", "onnxruntime"]:
        try: __import__(pkg)
        except ImportError: m.append(pkg.replace("cv2","opencv-python"))
    if m:
        print(f"❌ Missing: {', '.join(m)}\n   pip install {' '.join(m)}")
        sys.exit(1)
_check()

import cv2
import mediapipe as mp
import onnxruntime as ort

# ── Paths ──
BASE = os.path.dirname(os.path.abspath(__file__))
MODELS = os.path.join(BASE, "models")

# ── Color Palette ──
BG_DARK    = (18, 18, 24)
BG_PANEL   = (28, 28, 38)
ACCENT_ASL = (100, 220, 80)   # green
ACCENT_ISL = (80, 160, 255)   # blue
WHITE      = (240, 240, 240)
DIM        = (130, 130, 150)
HAND_COLOR = (0, 255, 200)
HAND_CONN  = (0, 180, 140)
WARN_RED   = (60, 60, 255)
GOLD       = (50, 215, 255)


# ═══════════════════════════════════════════
#  ASL Model (250 words, 543-landmark input)
# ═══════════════════════════════════════════
class ASLModel:
    def __init__(self):
        path = os.path.join(MODELS, "signify_model.onnx")
        labels_path = os.path.join(MODELS, "sign_to_prediction_index_map.json")
        self.session = ort.InferenceSession(path)
        self.input_name = self.session.get_inputs()[0].name
        with open(labels_path) as f:
            s2i = json.load(f)
        self.idx_to_sign = {v: k for k, v in s2i.items()}
        self.n_labels = len(self.idx_to_sign)

    def predict(self, landmarks_543x3):
        t = landmarks_543x3.astype(np.float32)
        if t.ndim == 2:
            t = t[np.newaxis]  # [1, 543, 3]
        try:
            out = self.session.run(None, {self.input_name: t})
        except:
            return []
        logits = out[0][0] if out[0].ndim > 1 else out[0]
        top = np.argsort(logits)[-5:][::-1]
        return [(self.idx_to_sign.get(i, self.idx_to_sign.get(str(i), f"?{i}")), float(logits[i])) for i in top]


# ═══════════════════════════════════════════
#  ISL Model (39 symbols, 84-dim hand input)
# ═══════════════════════════════════════════
class ISLModel:
    def __init__(self):
        path = os.path.join(MODELS, "signai_isl_classifier.onnx")
        labels_path = os.path.join(MODELS, "signai_isl_labels.json")
        self.session = ort.InferenceSession(path)
        self.input_name = self.session.get_inputs()[0].name
        self.output_name = self.session.get_outputs()[0].name
        with open(labels_path) as f:
            self.labels = json.load(f)

    def _norm(self, lms):
        pts = [(l[0], l[1]) for l in lms[:21]]
        bx, by = pts[0]
        flat = [c for x, y in pts for c in (x - bx, y - by)]
        mx = max(abs(v) for v in flat) or 1.0
        return [v / mx for v in flat]

    def predict(self, left=None, right=None):
        lf = self._norm(left) if left else [0.0]*42
        rf = self._norm(right) if right else [0.0]*42
        feat = np.array(lf + rf, dtype=np.float32).reshape(1, -1)
        res = self.session.run([self.output_name], {self.input_name: feat})
        probs = res[0][0]
        top = np.argsort(probs)[-5:][::-1]
        return [(self.labels[i] if i < len(self.labels) else f"?{i}", float(probs[i])) for i in top]


# ═══════════════════════════════════════════
#  Prediction Stabilizer (reduces flicker)
# ═══════════════════════════════════════════
class Stabilizer:
    def __init__(self, window=8, threshold=3):
        self.window = window
        self.threshold = threshold
        self.history = collections.deque(maxlen=window)
        self.current = None
        self.confidence = 0.0

    def update(self, predictions):
        if not predictions:
            return self.current, self.confidence
        top_label, top_conf = predictions[0]
        self.history.append(top_label)
        counter = collections.Counter(self.history)
        most_common, count = counter.most_common(1)[0]
        if count >= self.threshold:
            self.current = most_common
            self.confidence = top_conf
        return self.current, self.confidence

    def clear(self):
        self.history.clear()
        self.current = None
        self.confidence = 0.0


# ═══════════════════════════════════════════
#  MediaPipe → Landmarks
# ═══════════════════════════════════════════
def get_holistic_543(r):
    lm = np.zeros((543, 3), dtype=np.float32)
    if r.pose_landmarks:
        for i, p in enumerate(r.pose_landmarks.landmark):
            lm[i] = [p.x, p.y, p.z]
    if r.face_landmarks:
        for i, p in enumerate(r.face_landmarks.landmark):
            lm[33+i] = [p.x, p.y, p.z]
    if r.left_hand_landmarks:
        for i, p in enumerate(r.left_hand_landmarks.landmark):
            lm[501+i] = [p.x, p.y, p.z]
    if r.right_hand_landmarks:
        for i, p in enumerate(r.right_hand_landmarks.landmark):
            lm[522+i] = [p.x, p.y, p.z]
    return lm

def get_hands(r):
    L = [(p.x, p.y) for p in r.left_hand_landmarks.landmark] if r.left_hand_landmarks else None
    R = [(p.x, p.y) for p in r.right_hand_landmarks.landmark] if r.right_hand_landmarks else None
    return L, R

def has_hands(r):
    return r.left_hand_landmarks is not None or r.right_hand_landmarks is not None


# ═══════════════════════════════════════════
#  UI Drawing
# ═══════════════════════════════════════════
def rounded_rect(img, pt1, pt2, color, radius=12, thickness=-1):
    """Draw a filled rounded rectangle."""
    x1, y1 = pt1
    x2, y2 = pt2
    cv2.rectangle(img, (x1+radius, y1), (x2-radius, y2), color, thickness)
    cv2.rectangle(img, (x1, y1+radius), (x2, y2-radius), color, thickness)
    cv2.circle(img, (x1+radius, y1+radius), radius, color, thickness)
    cv2.circle(img, (x2-radius, y1+radius), radius, color, thickness)
    cv2.circle(img, (x1+radius, y2-radius), radius, color, thickness)
    cv2.circle(img, (x2-radius, y2-radius), radius, color, thickness)


def draw_hand_lm(frame, hand_landmarks, connections, w, h):
    """Draw nicer hand landmarks with glowing dots."""
    if not hand_landmarks:
        return
    pts = []
    for lm in hand_landmarks.landmark:
        px, py = int(lm.x * w), int(lm.y * h)
        pts.append((px, py))

    # Draw connections
    for c in connections:
        cv2.line(frame, pts[c[0]], pts[c[1]], HAND_CONN, 2, cv2.LINE_AA)

    # Draw landmarks
    for px, py in pts:
        cv2.circle(frame, (px, py), 5, HAND_COLOR, -1, cv2.LINE_AA)
        cv2.circle(frame, (px, py), 7, (0, 100, 80), 1, cv2.LINE_AA)


def draw_ui(frame, mode, predictions, stable_label, stable_conf, 
            hands_vis, fps, sentence):
    h, w = frame.shape[:2]
    accent = ACCENT_ASL if mode == "ASL" else ACCENT_ISL

    # ── Top bar ──
    overlay = frame.copy()
    cv2.rectangle(overlay, (0, 0), (w, 70), BG_DARK, -1)
    cv2.addWeighted(overlay, 0.85, frame, 0.15, 0, frame)

    # Mode badge
    badge_w = 75
    rounded_rect(frame, (12, 12), (12+badge_w, 48), accent, radius=8)
    cv2.putText(frame, mode, (22, 40), cv2.FONT_HERSHEY_SIMPLEX, 0.7, BG_DARK, 2, cv2.LINE_AA)

    # FPS
    cv2.putText(frame, f"{fps:.0f} FPS", (w-100, 38), cv2.FONT_HERSHEY_SIMPLEX, 0.5, DIM, 1, cv2.LINE_AA)

    # Hand status indicator
    dot_color = (0, 255, 100) if hands_vis else WARN_RED
    cv2.circle(frame, (badge_w + 30, 30), 6, dot_color, -1, cv2.LINE_AA)
    status_txt = "HANDS" if hands_vis else "NO HANDS"
    cv2.putText(frame, status_txt, (badge_w + 42, 36), cv2.FONT_HERSHEY_SIMPLEX, 0.4, dot_color, 1, cv2.LINE_AA)

    # Controls hint
    cv2.putText(frame, "q:quit  s:switch  c:clear  space:freeze", (12, 62), 
                cv2.FONT_HERSHEY_SIMPLEX, 0.35, DIM, 1, cv2.LINE_AA)

    # ── Stable prediction (big, center-top) ──
    if stable_label:
        display_label = stable_label.upper()
        font_scale = 2.0 if len(display_label) <= 3 else 1.2
        (tw, th), _ = cv2.getTextSize(display_label, cv2.FONT_HERSHEY_SIMPLEX, font_scale, 3)
        tx = (w - tw) // 2
        # Shadow
        cv2.putText(frame, display_label, (tx+2, 122), cv2.FONT_HERSHEY_SIMPLEX, font_scale, (0,0,0), 4, cv2.LINE_AA)
        # Text
        cv2.putText(frame, display_label, (tx, 120), cv2.FONT_HERSHEY_SIMPLEX, font_scale, accent, 3, cv2.LINE_AA)
        # Confidence bar
        bar_w = int(min(stable_conf * 2 * 200, 200))  # scale appropriately
        bar_x = (w - 200) // 2
        cv2.rectangle(frame, (bar_x, 130), (bar_x + 200, 138), (50, 50, 60), -1)
        cv2.rectangle(frame, (bar_x, 130), (bar_x + bar_w, 138), accent, -1)

    # ── Sentence bar (bottom-center) ──
    if sentence:
        sent_text = " ".join(sentence[-8:])  # last 8 words
        overlay2 = frame.copy()
        cv2.rectangle(overlay2, (0, h-50), (w, h), BG_DARK, -1)
        cv2.addWeighted(overlay2, 0.85, frame, 0.15, 0, frame)
        cv2.putText(frame, sent_text, (15, h-18), cv2.FONT_HERSHEY_SIMPLEX, 0.65, GOLD, 2, cv2.LINE_AA)

    # ── Predictions panel (right side) ──
    if predictions:
        panel_x = w - 220
        panel_y = 80
        overlay3 = frame.copy()
        cv2.rectangle(overlay3, (panel_x - 10, panel_y - 5), (w, panel_y + 135), BG_PANEL, -1)
        cv2.addWeighted(overlay3, 0.75, frame, 0.25, 0, frame)

        cv2.putText(frame, "Top 5", (panel_x, panel_y + 14), cv2.FONT_HERSHEY_SIMPLEX, 0.4, DIM, 1, cv2.LINE_AA)

        for i, (label, conf) in enumerate(predictions[:5]):
            y = panel_y + 38 + i * 22
            color = accent if i == 0 else DIM
            thick = 1
            cv2.putText(frame, f"{label:>10s}", (panel_x, y), cv2.FONT_HERSHEY_SIMPLEX, 0.45, color, thick, cv2.LINE_AA)
            # Mini bar
            bw = int(min(abs(conf) * 30, 60))
            cv2.rectangle(frame, (panel_x + 90, y - 8), (panel_x + 90 + bw, y - 2), color, -1)


# ═══════════════════════════════════════════
#  Main
# ═══════════════════════════════════════════
def main():
    print("\n" + "═" * 55)
    print("  🤟 SIGNIFY — Model Test Mode")
    print("═" * 55)

    # Check files
    asl_ok = os.path.exists(os.path.join(MODELS, "signify_model.onnx"))
    isl_ok = os.path.exists(os.path.join(MODELS, "signai_isl_classifier.onnx"))
    print(f"\n  ASL model: {'✅' if asl_ok else '❌'}   ISL model: {'✅' if isl_ok else '❌'}")
    if not asl_ok and not isl_ok:
        print("  No models found!"); sys.exit(1)

    # Ask mode
    print("\n  Select mode:")
    if asl_ok and isl_ok:
        print("    1) ASL — American Sign Language (250 words)")
        print("    2) ISL — Indian Sign Language (39 symbols)")
        choice = input("\n  Enter 1 or 2: ").strip()
        mode = "ISL" if choice == "2" else "ASL"
    elif asl_ok:
        mode = "ASL"
    else:
        mode = "ISL"

    # Load model
    print(f"\n  Loading {mode}...")
    asl_m = ASLModel() if mode == "ASL" else None
    isl_m = ISLModel() if mode == "ISL" else None
    print(f"  ✅ {mode} ready\n")

    # MediaPipe
    mp_holistic = mp.solutions.holistic
    cap = cv2.VideoCapture(0)
    if not cap.isOpened():
        print("  ❌ Cannot open webcam"); sys.exit(1)
    cap.set(cv2.CAP_PROP_FRAME_WIDTH, 640)
    cap.set(cv2.CAP_PROP_FRAME_HEIGHT, 480)

    print("  ✅ Webcam ready — show your signs!")
    print("  ─" * 25)

    stabilizer = Stabilizer(window=8, threshold=3)
    predictions = []
    sentence = []
    last_word = None
    prev_time = time.time()
    fps = 30.0
    freeze = False

    with mp_holistic.Holistic(
        static_image_mode=False,
        model_complexity=1,
        min_detection_confidence=0.5,
        min_tracking_confidence=0.5,
    ) as holistic:

        while cap.isOpened():
            if not freeze:
                ret, frame = cap.read()
                if not ret: break
                frame = cv2.flip(frame, 1)
            else:
                # Keep showing frozen frame
                pass

            h, w = frame.shape[:2]

            # Process
            if not freeze:
                rgb = cv2.cvtColor(frame, cv2.COLOR_BGR2RGB)
                results = holistic.process(rgb)

                # FPS
                now = time.time()
                fps = 0.9 * fps + 0.1 / max(now - prev_time, 0.001)
                prev_time = now

                # Draw hands
                if results.left_hand_landmarks:
                    draw_hand_lm(frame, results.left_hand_landmarks, mp_holistic.HAND_CONNECTIONS, w, h)
                if results.right_hand_landmarks:
                    draw_hand_lm(frame, results.right_hand_landmarks, mp_holistic.HAND_CONNECTIONS, w, h)

                # Predict
                hv = has_hands(results)
                if hv:
                    try:
                        if mode == "ASL" and asl_m:
                            lm = get_holistic_543(results)
                            predictions = asl_m.predict(lm)
                        elif mode == "ISL" and isl_m:
                            L, R = get_hands(results)
                            predictions = isl_m.predict(L, R)
                    except:
                        predictions = []

                    # Stabilize
                    stable_label, stable_conf = stabilizer.update(predictions)

                    # Sentence building (add word when stable and different from last)
                    if stable_label and stable_label != last_word:
                        # Filter out noise labels
                        if stable_label not in ("~", "!", "fullstop"):
                            sentence.append(stable_label)
                            last_word = stable_label
                else:
                    stable_label, stable_conf = stabilizer.current, stabilizer.confidence

            # Draw UI
            draw_ui(frame, mode, predictions, 
                    stabilizer.current, stabilizer.confidence,
                    hv if not freeze else True, fps, sentence)

            if freeze:
                cv2.putText(frame, "PAUSED", (w//2 - 60, h//2), 
                           cv2.FONT_HERSHEY_SIMPLEX, 1.0, WARN_RED, 2, cv2.LINE_AA)

            cv2.imshow("Signify Test Mode", frame)

            # Keys
            key = cv2.waitKey(1) & 0xFF
            if key == ord('q'):
                break
            elif key == ord('c'):
                predictions = []
                stabilizer.clear()
                sentence = []
                last_word = None
            elif key == ord(' '):
                freeze = not freeze
            elif key == ord('s'):
                new_mode = "ISL" if mode == "ASL" else "ASL"
                if (new_mode == "ASL" and asl_ok) or (new_mode == "ISL" and isl_ok):
                    mode = new_mode
                    if mode == "ASL" and asl_m is None:
                        print(f"  Loading ASL...")
                        asl_m = ASLModel()
                    if mode == "ISL" and isl_m is None:
                        print(f"  Loading ISL...")
                        isl_m = ISLModel()
                    predictions = []
                    stabilizer.clear()
                    sentence = []
                    last_word = None
                    print(f"  → Switched to {mode}")

    cap.release()
    cv2.destroyAllWindows()

    # Print final sentence
    if sentence:
        print(f"\n  📝 Sentence: {' '.join(sentence)}")
    print("  👋 Done.")


if __name__ == "__main__":
    main()
