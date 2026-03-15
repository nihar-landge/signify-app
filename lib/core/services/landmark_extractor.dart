import 'dart:ui' show Size;
import 'package:camera/camera.dart';
import 'package:flutter/foundation.dart';
import 'package:google_mlkit_pose_detection/google_mlkit_pose_detection.dart';
import 'package:hand_landmarker/hand_landmarker.dart';

/// Represents a single landmark point with x, y, z coordinates
class LandmarkPoint {
  final double x;
  final double y;
  final double z;

  const LandmarkPoint(this.x, this.y, this.z);

  List<double> toList() => [x, y, z];
}

/// Frame of landmark data extracted from a camera frame
class LandmarkFrame {
  final int timestamp;
  final int sequence;
  final List<LandmarkPoint>? leftHand; // 21 points (null if not detected)
  final List<LandmarkPoint>? rightHand; // 21 points (null if not detected)
  final List<LandmarkPoint>? pose; // 33 points
  final List<LandmarkPoint>? face; // key face points

  LandmarkFrame({
    required this.timestamp,
    required this.sequence,
    this.leftHand,
    this.rightHand,
    this.pose,
    this.face,
  });

  bool get hasData =>
      (leftHand != null && leftHand!.isNotEmpty) ||
      (rightHand != null && rightHand!.isNotEmpty) ||
      (pose != null && pose!.isNotEmpty);

  // P7 fix: reuse a single pre-allocated 1629-element buffer so the GC is not
  // hit with two large List allocations on every camera frame (~25/sec).
  static final _holisticBuffer =
      List<double>.filled(1629, 0.0, growable: false);

  /// Build the 1629-float array expected by the backend.
  /// Order: Pose (33×3=99) + Face (468×3=1404) + LeftHand (21×3=63) + RightHand (21×3=63)
  /// Sparse/missing landmarks are zero-filled.
  /// Returns a copy of the shared buffer — safe to hold across async boundaries.
  List<double> toFullHolisticLandmarks() {
    final out = _holisticBuffer;
    int i = 0;

    // 1. Pose (33 × 3 = 99)
    if (pose != null && pose!.length == 33) {
      for (final p in pose!) {
        out[i++] = p.x;
        out[i++] = p.y;
        out[i++] = p.z;
      }
    } else {
      for (int k = 0; k < 99; k++) {
        out[i++] = 0.0;
      }
    }

    // 2. Face (468 × 3 = 1404) — ML Kit only gives a few key points,
    //    so we zero-fill the full 468-slot block.
    for (int k = 0; k < 1404; k++) {
      out[i++] = 0.0;
    }

    // 3. Left hand (21 × 3 = 63)
    if (leftHand != null && leftHand!.length == 21) {
      for (final p in leftHand!) {
        out[i++] = p.x;
        out[i++] = p.y;
        out[i++] = p.z;
      }
    } else {
      for (int k = 0; k < 63; k++) {
        out[i++] = 0.0;
      }
    }

    // 4. Right hand (21 × 3 = 63)
    if (rightHand != null && rightHand!.length == 21) {
      for (final p in rightHand!) {
        out[i++] = p.x;
        out[i++] = p.y;
        out[i++] = p.z;
      }
    } else {
      for (int k = 0; k < 63; k++) {
        out[i++] = 0.0;
      }
    }

    assert(i == 1629, 'Expected 1629 floats, wrote $i');
    // Return a copy so the next frame's toFullHolisticLandmarks() call cannot
    // mutate the shared buffer while a previous packet's jsonEncode() is still
    // iterating it inside a .then() async callback.
    return List<double>.of(out);
  }
}

/// Service for extracting pose + hand landmarks from camera frames.
///
/// - Pose (33 pts): ML Kit PoseDetector in stream mode.
/// - Face (468 pts): zero-filled (face mesh not needed by the backend model).
/// - Hands (21 pts each): MediaPipe HandLandmarker via the hand_landmarker
///   plugin (GPU-accelerated JNI bridge, Android only).
///
/// The ONNX model inference runs server-side; Flutter only extracts and
/// streams the landmark coordinates.
class LandmarkExtractor {
  late final PoseDetector _poseDetector;
  late final HandLandmarkerPlugin _handLandmarker;

  bool _isInitialized = false;
  int _sequenceNumber = 0;
  bool _isProcessing = false;

  bool get isInitialized => _isInitialized;

  Future<void> initialize() async {
    if (_isInitialized) return;

    _poseDetector = PoseDetector(
      options: PoseDetectorOptions(
        mode: PoseDetectionMode.stream,
        model: PoseDetectionModel.base,
      ),
    );

    // GPU delegate for best performance; falls back to CPU automatically on
    // devices where GPU is unavailable.
    _handLandmarker = HandLandmarkerPlugin.create(
      numHands: 2,
      minHandDetectionConfidence: 0.5,
      delegate: HandLandmarkerDelegate.GPU,
    );

    _isInitialized = true;
    debugPrint('LandmarkExtractor initialized (with HandLandmarkerPlugin)');
  }

  /// Process a camera frame and extract landmarks
  Future<LandmarkFrame?> processImage(
    CameraImage cameraImage,
    CameraDescription camera,
  ) async {
    if (!_isInitialized) return null;
    if (_isProcessing) return null;
    _isProcessing = true;

    try {
      final inputImage = _convertCameraImage(cameraImage, camera);
      if (inputImage == null) return null;

      // ML Kit pose detection (async)
      final poses = await _poseDetector.processImage(inputImage);

      // MediaPipe hand landmark detection — wrapped in Future() to avoid
      // blocking the platform/UI thread. The synchronous JNI call would
      // otherwise starve the camera image stream and trigger an ANR.
      List<Hand> hands = [];
      try {
        hands = await Future(
          () => _handLandmarker.detect(cameraImage, camera.sensorOrientation),
        );
      } catch (e) {
        debugPrint('LandmarkExtractor: HandLandmarker error: $e');
        // Non-fatal — continue with zero-filled hand slots
      }

      final frame = _extractLandmarks(
        poses,
        hands,
        cameraImage.width,
        cameraImage.height,
      );

      if (frame.hasData) {
        // Frame is returned directly to the caller (call_screen.dart) via the
        // Future return value. The broadcast stream was unused (P15 fix).
      }
      return frame;
    } catch (e) {
      debugPrint('LandmarkExtractor: Error processing image: $e');
      return null;
    } finally {
      _isProcessing = false;
    }
  }

  InputImage? _convertCameraImage(
    CameraImage cameraImage,
    CameraDescription camera,
  ) {
    try {
      final rotation = _getImageRotation(camera);
      if (rotation == null) return null;

      final format = InputImageFormatValue.fromRawValue(cameraImage.format.raw);
      if (format == null) return null;

      final bytesPerRow = cameraImage.planes.first.bytesPerRow;

      final allBytes = WriteBuffer();
      for (final plane in cameraImage.planes) {
        allBytes.putUint8List(plane.bytes);
      }
      final bytes = allBytes.done().buffer.asUint8List();

      return InputImage.fromBytes(
        bytes: bytes,
        metadata: InputImageMetadata(
          size: Size(
            cameraImage.width.toDouble(),
            cameraImage.height.toDouble(),
          ),
          rotation: rotation,
          format: format,
          bytesPerRow: bytesPerRow,
        ),
      );
    } catch (e) {
      debugPrint('LandmarkExtractor: Error converting camera image: $e');
      return null;
    }
  }

  InputImageRotation? _getImageRotation(CameraDescription camera) {
    switch (camera.sensorOrientation) {
      case 0:
        return InputImageRotation.rotation0deg;
      case 90:
        return InputImageRotation.rotation90deg;
      case 180:
        return InputImageRotation.rotation180deg;
      case 270:
        return InputImageRotation.rotation270deg;
      default:
        return null;
    }
  }

  LandmarkFrame _extractLandmarks(
    List<Pose> poses,
    List<Hand> hands,
    int imageWidth,
    int imageHeight,
  ) {
    _sequenceNumber++;

    // --- Pose ---
    List<LandmarkPoint>? posePoints;
    if (poses.isNotEmpty) {
      final pose = poses.first;
      posePoints = [];
      for (final type in PoseLandmarkType.values) {
        final landmark = pose.landmarks[type];
        if (landmark != null) {
          posePoints.add(LandmarkPoint(
            landmark.x / imageWidth,
            landmark.y / imageHeight,
            landmark.z / 1000.0,
          ));
        }
      }
    }

    // --- Hands ---
    // The hand_landmarker plugin does not expose handedness, so we use the
    // wrist x-coordinate to assign left/right:
    //   • For a front-facing camera the image is mirrored, so the hand with
    //     the SMALLER wrist-x (further left on screen) maps to the user's
    //     right hand in world-space — however the backend landmark format
    //     follows screen-left = LeftHand / screen-right = RightHand (same
    //     convention as MediaPipe Holistic in image space).
    //   • We therefore simply assign: lowest wrist-x → leftHand slot,
    //     highest wrist-x → rightHand slot, which matches what the backend
    //     expects for the spatial positions of the hands.
    List<LandmarkPoint>? leftHand;
    List<LandmarkPoint>? rightHand;

    if (hands.isNotEmpty) {
      Hand? leftCandidate;
      Hand? rightCandidate;

      if (hands.length == 1) {
        final wristX = hands[0].landmarks[0].x;
        // Place single hand in the correct slot based on screen position
        if (wristX < 0.5) {
          leftCandidate = hands[0];
        } else {
          rightCandidate = hands[0];
        }
      } else {
        // Two (or more) hands — sort by wrist x-coordinate
        final sorted = List<Hand>.from(hands)
          ..sort((a, b) => a.landmarks[0].x.compareTo(b.landmarks[0].x));
        leftCandidate = sorted.first;
        rightCandidate = sorted.last;
      }

      leftHand = _handToLandmarkPoints(leftCandidate);
      rightHand = _handToLandmarkPoints(rightCandidate);
    }

    return LandmarkFrame(
      timestamp: DateTime.now().millisecondsSinceEpoch,
      sequence: _sequenceNumber,
      pose: posePoints,
      face: null, // 468-pt face mesh not available via ML Kit
      leftHand: leftHand,
      rightHand: rightHand,
    );
  }

  /// Convert a [Hand] from the plugin into our 21-point [LandmarkPoint] list.
  /// Returns null if the hand is null or doesn't have exactly 21 landmarks.
  List<LandmarkPoint>? _handToLandmarkPoints(Hand? hand) {
    if (hand == null || hand.landmarks.length != 21) return null;
    return hand.landmarks.map((l) => LandmarkPoint(l.x, l.y, l.z)).toList();
  }

  void resetSequence() {
    _sequenceNumber = 0;
  }

  Future<void> dispose() async {
    await _poseDetector.close();
    _handLandmarker.dispose();
    _isInitialized = false;
    debugPrint('LandmarkExtractor disposed');
  }
}

/// Singleton instance
final landmarkExtractor = LandmarkExtractor();
