import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// Represents a single landmark point with x, y, z coordinates
class LandmarkPoint {
  final double x;
  final double y;
  final double z;

  const LandmarkPoint(this.x, this.y, this.z);

  List<double> toList() => [x, y, z];

  factory LandmarkPoint.fromList(List<dynamic> coords) {
    return LandmarkPoint(
      (coords[0] as num).toDouble(),
      (coords[1] as num).toDouble(),
      (coords[2] as num).toDouble(),
    );
  }
}

/// Complete frame of holistic landmark data (543 points total)
class HolisticLandmarkFrame {
  final int timestamp;
  final int sequence;
  final List<LandmarkPoint>? pose; // 33 points
  final List<LandmarkPoint>? leftHand; // 21 points
  final List<LandmarkPoint>? rightHand; // 21 points
  final List<LandmarkPoint>? face; // 468 points

  HolisticLandmarkFrame({
    required this.timestamp,
    required this.sequence,
    this.pose,
    this.leftHand,
    this.rightHand,
    this.face,
  });

  bool get hasData =>
      (pose != null && pose!.isNotEmpty) ||
      (leftHand != null && leftHand!.isNotEmpty) ||
      (rightHand != null && rightHand!.isNotEmpty) ||
      (face != null && face!.isNotEmpty);

  bool get hasHands =>
      (leftHand != null && leftHand!.isNotEmpty) ||
      (rightHand != null && rightHand!.isNotEmpty);

  /// Create from native platform response
  factory HolisticLandmarkFrame.fromMap(Map<dynamic, dynamic> map) {
    return HolisticLandmarkFrame(
      timestamp: map['ts'] as int? ?? DateTime.now().millisecondsSinceEpoch,
      sequence: map['seq'] as int? ?? 0,
      pose: _parsePointList(map['pose']),
      leftHand: _parsePointList(map['lh']),
      rightHand: _parsePointList(map['rh']),
      face: _parsePointList(map['face']),
    );
  }

  static List<LandmarkPoint>? _parsePointList(dynamic data) {
    if (data == null || data is! List) return null;
    return data.map((coords) {
      if (coords is List && coords.length >= 3) {
        return LandmarkPoint.fromList(coords);
      }
      return const LandmarkPoint(0, 0, 0);
    }).toList();
  }

  /// Extract full holistic landmarks as 1629 floats
  /// Order: Pose (33×3) + Face (468×3) + LeftHand (21×3) + RightHand (21×3)
  /// Missing landmarks are filled with zeros
  List<double> extractFullHolisticLandmarks() {
    final landmarks = <double>[];

    // 1. Pose landmarks (33 × 3 = 99 floats)
    if (pose != null && pose!.length == 33) {
      for (final point in pose!) {
        landmarks.addAll(point.toList());
      }
    } else {
      landmarks.addAll(List.filled(99, 0.0));
    }

    // 2. Face landmarks (468 × 3 = 1404 floats)
    if (face != null && face!.length == 468) {
      for (final point in face!) {
        landmarks.addAll(point.toList());
      }
    } else {
      landmarks.addAll(List.filled(1404, 0.0));
    }

    // 3. Left hand landmarks (21 × 3 = 63 floats)
    if (leftHand != null && leftHand!.length == 21) {
      for (final point in leftHand!) {
        landmarks.addAll(point.toList());
      }
    } else {
      landmarks.addAll(List.filled(63, 0.0));
    }

    // 4. Right hand landmarks (21 × 3 = 63 floats)
    if (rightHand != null && rightHand!.length == 21) {
      for (final point in rightHand!) {
        landmarks.addAll(point.toList());
      }
    } else {
      landmarks.addAll(List.filled(63, 0.0));
    }

    // Total: 99 + 1404 + 63 + 63 = 1629 floats
    assert(
      landmarks.length == 1629,
      'Expected 1629 floats, got ${landmarks.length}',
    );
    return landmarks;
  }
}

/// Service for MediaPipe Holistic landmark extraction via platform channel
class MediaPipeHolisticService {
  static const _channel = MethodChannel('com.signify.mediapipe/holistic');

  bool _isInitialized = false;
  bool get isInitialized => _isInitialized;

  bool _isProcessing = false;

  final _landmarkController =
      StreamController<HolisticLandmarkFrame>.broadcast();
  Stream<HolisticLandmarkFrame> get landmarkStream =>
      _landmarkController.stream;

  /// Initialize MediaPipe Holistic (native side)
  Future<bool> initialize() async {
    if (_isInitialized) return true;

    try {
      final result = await _channel.invokeMethod<bool>('initialize');
      _isInitialized = result ?? false;

      if (_isInitialized) {
        debugPrint('MediaPipe Holistic initialized successfully');
      } else {
        debugPrint('Failed to initialize MediaPipe Holistic');
      }

      return _isInitialized;
    } on PlatformException catch (e) {
      debugPrint('Failed to initialize MediaPipe: ${e.message}');
      _isInitialized = false;
      return false;
    }
  }

  /// Process an image and extract all 543 landmarks
  Future<HolisticLandmarkFrame?> processImage(
    Uint8List imageBytes,
    int width,
    int height, {
    int rotation = 0,
  }) async {
    if (!_isInitialized) return null;
    if (_isProcessing) return null;
    _isProcessing = true;

    try {
      final result = await _channel.invokeMethod<Map<dynamic, dynamic>>(
        'processImage',
        {
          'imageBytes': imageBytes,
          'width': width,
          'height': height,
          'rotation': rotation,
        },
      );

      if (result == null) return null;

      final frame = HolisticLandmarkFrame.fromMap(result);
      if (frame.hasData) {
        _landmarkController.add(frame);
      }
      return frame;
    } on PlatformException catch (e) {
      debugPrint('Error processing image: ${e.message}');
      return null;
    } finally {
      _isProcessing = false;
    }
  }

  /// Reset sequence counter (call when starting new session)
  Future<void> resetSequence() async {
    try {
      await _channel.invokeMethod('resetSequence');
    } catch (e) {
      debugPrint('Error resetting sequence: $e');
    }
  }

  Future<void> dispose() async {
    try {
      await _channel.invokeMethod('dispose');
    } catch (e) {
      debugPrint('Error disposing MediaPipe: $e');
    }
    await _landmarkController.close();
    _isInitialized = false;
  }
}

/// Singleton instance
final mediaPipeHolistic = MediaPipeHolisticService();
