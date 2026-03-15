import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
import 'package:web_socket_channel/io.dart';

/// V1 Landmark packet for streaming to backend
class LandmarkPacket {
  final String sessionId;
  final int seq;
  final int tsMs;
  final bool present;
  final List<double>? landmarks; // 1629 floats when present=true
  final bool normalized;

  LandmarkPacket({
    required this.sessionId,
    required this.seq,
    required this.tsMs,
    required this.present,
    this.landmarks,
    this.normalized = true,
  });

  Map<String, dynamic> toJson() => {
        'sessionId': sessionId,
        'seq': seq,
        'ts_ms': tsMs,
        'present': present,
        'landmarks': present ? landmarks : [],
        'normalized': normalized,
      };

  String toJsonString() => jsonEncode(toJson());
}

/// WebSocket client for streaming hand landmarks to the backend
class LandmarkWebSocketClient {
  final String serverUrl;
  String sessionId;
  final String requestLanguage;

  WebSocketChannel? _channel;
  bool _isConnected = false;
  bool _disposed = false;
  int _seq = 0;
  Timer? _heartbeatTimer;
  Timer? _reconnectTimer;

  // Stats
  int _sentCount = 0;
  int _dropCount = 0;

  // Rate limiting
  DateTime? _lastSendTime;
  static const _minSendInterval = Duration(milliseconds: 40); // ~25 FPS max

  // Callbacks
  Function(String text, bool isFinal)? onSignEvent;
  Function(Uint8List audio)? onAudioReceived;
  Function(String type, String message)? onError;
  Function()? onConnected;
  Function()? onDisconnected;

  LandmarkWebSocketClient({
    required this.serverUrl,
    required this.sessionId,
    this.requestLanguage = 'ASL',
  });

  bool get isConnected => _isConnected;
  int get sentCount => _sentCount;
  int get dropCount => _dropCount;

  /// Connect to the landmarks WebSocket endpoint
  Future<void> connect() async {
    if (_isConnected || _disposed) return;

    try {
      final wsUrl =
          '$serverUrl/ws/app/landmarks?sessionId=$sessionId&language=$requestLanguage';
      debugPrint('LandmarkWS: Connecting to $wsUrl');

      _channel = IOWebSocketChannel.connect(Uri.parse(wsUrl).removeFragment());
      // Guard against concurrent disconnect() nulling _channel before ready.
      final channel = _channel;
      if (channel == null || _disposed) return;
      await channel.ready.timeout(const Duration(seconds: 5));

      if (_disposed) {
        await channel.sink.close();
        return;
      }

      _isConnected = true;
      _seq = 0;
      _sentCount = 0;
      _dropCount = 0;

      channel.stream.listen(
        _handleMessage,
        onError: _handleError,
        onDone: _handleDisconnect,
      );

      _startHeartbeat();
      debugPrint('LandmarkWS: Connected');
      onConnected?.call();
    } catch (e) {
      debugPrint('LandmarkWS: Connection failed: $e');
      onError?.call('connection', e.toString());
      _scheduleReconnect();
    }
  }

  /// Send a landmark packet
  void sendLandmarks({required bool present, List<double>? landmarks}) {
    if (!_isConnected || _channel == null) {
      _dropCount++;
      return;
    }

    // Rate limit check
    final now = DateTime.now();
    if (_lastSendTime != null &&
        now.difference(_lastSendTime!) < _minSendInterval) {
      _dropCount++;
      return;
    }
    _lastSendTime = now;

    // Validate landmarks (full holistic: 543 landmarks × 3 = 1629 floats)
    if (present && (landmarks == null || landmarks.length != 1629)) {
      debugPrint(
        'LandmarkWS: REJECT landmarks length: ${landmarks?.length}, expected 1629',
      );
      return;
    }

    final packet = LandmarkPacket(
      sessionId: sessionId,
      seq: _seq++,
      tsMs: now.millisecondsSinceEpoch,
      present: present,
      landmarks: present ? landmarks : null,
      normalized: true,
    );

    try {
      _channel!.sink.add(packet.toJsonString());
      _sentCount++;
    } catch (e) {
      debugPrint('LandmarkWS: Send error: $e');
      _dropCount++;
    }
  }

  /// Disconnect from the server
  void disconnect() {
    _disposed = true;
    _heartbeatTimer?.cancel();
    _reconnectTimer?.cancel();
    _channel?.sink.close();
    _channel = null;
    _isConnected = false;
    debugPrint('LandmarkWS: Disconnected');
    onDisconnected?.call();
  }

  void dispose() => disconnect();

  /// Update sessionId dynamically (reconnects if currently connected)
  void updateSessionId(String newId) {
    if (sessionId == newId) return;
    debugPrint('LandmarkWS: Updating sessionId from $sessionId to $newId');
    sessionId = newId;
    if (_isConnected) {
      _disposed = false; // allow reconnect
      disconnect();
      connect();
    }
  }

  void _handleMessage(dynamic message) {
    if (message is! String) return;
    try {
      final data = jsonDecode(message) as Map<String, dynamic>;
      final type = data['type'] as String?;

      switch (type) {
        case 'sign_partial':
        case 'sign_final':
          final text = data['text'] as String?;
          if (text != null) {
            final filteredText = _filterByLanguage(text);
            if (filteredText != null) {
              onSignEvent?.call(filteredText, type == 'sign_final');
            }
          }
          final audioBase64 = data['audio'] as String?;
          if (audioBase64 != null && audioBase64.isNotEmpty) {
            try {
              onAudioReceived?.call(base64Decode(audioBase64));
            } catch (e) {
              debugPrint('LandmarkWS: Failed to decode audio base64: $e');
            }
          }
          break;
        case 'error':
          onError?.call(
              'server', data['message'] as String? ?? 'Unknown error');
          break;
        default:
          debugPrint('LandmarkWS: Unknown message type: $type');
      }
    } catch (e) {
      debugPrint('LandmarkWS: Failed to parse message: $e');
    }
  }

  void _handleError(dynamic error) {
    debugPrint('LandmarkWS: Stream error: $error');
    _isConnected = false;
    onError?.call('stream', error.toString());
    _scheduleReconnect();
  }

  void _handleDisconnect() {
    debugPrint('LandmarkWS: Connection closed');
    _isConnected = false;
    onDisconnected?.call();
    _scheduleReconnect();
  }

  void _startHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = Timer.periodic(const Duration(seconds: 15), (_) {
      if (_isConnected && _channel != null) {
        sendLandmarks(present: false);
      }
    });
  }

  void _scheduleReconnect() {
    if (_disposed) return;
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(const Duration(seconds: 2), () {
      if (!_isConnected && !_disposed) {
        debugPrint('LandmarkWS: Attempting reconnect...');
        connect();
      }
    });
  }

  Map<String, dynamic> getStats() => {
        'connected': _isConnected,
        'seq': _seq,
        'sent': _sentCount,
        'dropped': _dropCount,
      };

  /// Filter sign text by language preference.
  String? _filterByLanguage(String text) {
    if (text.startsWith('ASL:')) {
      return requestLanguage == 'ASL' ? text.substring(4) : null;
    } else if (text.startsWith('ISL:')) {
      return requestLanguage == 'ISL' ? text.substring(4) : null;
    }
    return text;
  }
}
