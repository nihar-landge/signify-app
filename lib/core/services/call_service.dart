import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:web_socket_channel/web_socket_channel.dart';
import 'package:web_socket_channel/io.dart';
import 'websocket_service.dart';

/// Represents a caption item received during a call
class CallCaption {
  final String text;
  final bool isFinal;
  final bool isFromSigner; // true = sign→text, false = speech→text
  final DateTime timestamp;

  CallCaption({
    required this.text,
    required this.isFinal,
    required this.isFromSigner,
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now();
}

/// Manages a single call session:
///   - POST /api/sessions/start → sessionId
///   - ws/app/events?sessionId=  → captions
///   - ws/app/landmarks?sessionId= → send landmarks during call
class CallSessionService {
  String? _sessionId;
  WebSocketChannel? _eventsChannel;
  bool _isConnected = false;

  // Stream controllers
  final _captionController = StreamController<CallCaption>.broadcast();
  final _statusController = StreamController<String>.broadcast();
  final _errorController = StreamController<String>.broadcast();
  final _audioController = StreamController<Uint8List>.broadcast();

  Stream<CallCaption> get captionStream => _captionController.stream;
  Stream<String> get statusStream => _statusController.stream;
  Stream<String> get errorStream => _errorController.stream;

  /// Emits decoded TTS audio bytes whenever a sign_final event contains audio.
  Stream<Uint8List> get audioStream => _audioController.stream;

  String? get sessionId => _sessionId;
  bool get isConnected => _isConnected;

  /// Start a new outbound call.
  /// Returns the sessionId on success, null on failure.
  Future<String?> startCall(String toPhoneNumber) async {
    final url = '${SignifyConfig.apiBaseUrl}/api/sessions/start';
    debugPrint('CallService: POST $url to=$toPhoneNumber');

    try {
      final response = await http
          .post(
            Uri.parse(url),
            headers: {'Content-Type': 'application/json'},
            body: jsonEncode({'to': toPhoneNumber}),
          )
          .timeout(const Duration(seconds: 10));

      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = jsonDecode(response.body) as Map<String, dynamic>;
        _sessionId = body['sessionId'] as String?;
        debugPrint('CallService: Session started: $_sessionId');
        if (_sessionId != null) {
          await _connectEventsWs(_sessionId!);
        }
        return _sessionId;
      } else {
        debugPrint(
          'CallService: startCall failed: ${response.statusCode} ${response.body}',
        );
        _errorController.add('Failed to start call: ${response.statusCode}');
        return null;
      }
    } catch (e) {
      debugPrint('CallService: startCall error: $e');
      _errorController.add('Network error: $e');
      return null;
    }
  }

  /// Accept an incoming call with a known sessionId.
  Future<void> acceptCall(String sessionId) async {
    _sessionId = sessionId;
    debugPrint('CallService: Accepting incoming call sessionId=$sessionId');
    await _connectEventsWs(sessionId);
  }

  /// Connect to ws/app/events for this session
  Future<void> _connectEventsWs(String sessionId) async {
    final wsUrl = '${SignifyConfig.rootUrl}/ws/app/events?sessionId=$sessionId';
    debugPrint('CallService: Connecting events WS: $wsUrl');

    try {
      _eventsChannel =
          IOWebSocketChannel.connect(Uri.parse(wsUrl).removeFragment());
      await _eventsChannel!.ready.timeout(const Duration(seconds: 5));
      _isConnected = true;

      _eventsChannel!.stream.listen(
        _handleEvent,
        onError: (e) {
          debugPrint('CallService: Events WS error: $e');
          _isConnected = false;
          _errorController.add('Connection error: $e');
        },
        onDone: () {
          debugPrint('CallService: Events WS closed');
          _isConnected = false;
        },
      );

      debugPrint('CallService: Events WS connected');
    } catch (e) {
      debugPrint('CallService: Events WS connect failed: $e');
      _isConnected = false;
      _errorController.add('Failed to connect: $e');
    }
  }

  void _handleEvent(dynamic message) {
    if (message is! String) return;
    try {
      final data = jsonDecode(message) as Map<String, dynamic>;
      final type = data['type'] as String? ?? '';
      final text = data['text'] as String? ?? '';

      switch (type) {
        case 'stt_partial':
          _captionController.add(
            CallCaption(text: text, isFinal: false, isFromSigner: false),
          );
          break;
        case 'stt_final':
          _captionController.add(
            CallCaption(text: text, isFinal: true, isFromSigner: false),
          );
          break;
        case 'sign_partial':
          _captionController.add(
            CallCaption(text: text, isFinal: false, isFromSigner: true),
          );
          break;
        case 'sign_final':
          _captionController.add(
            CallCaption(text: text, isFinal: true, isFromSigner: true),
          );
          // Extract TTS audio if present (test mode: base64 WAV/PCM bytes)
          final audioB64 = data['audio'] as String?;
          if (audioB64 != null && audioB64.isNotEmpty) {
            try {
              _audioController.add(base64Decode(audioB64));
            } catch (e) {
              debugPrint('CallService: Failed to decode audio: $e');
            }
          }
          break;
        case 'call_status':
          final status = data['status'] as String? ?? text;
          debugPrint('CallService: call_status=$status');
          _statusController.add(status);
          break;
        case 'error':
          final msg = data['message'] as String? ?? text;
          debugPrint('CallService: error event: $msg');
          _errorController.add(msg);
          break;
        default:
          debugPrint('CallService: Unknown event type: $type');
      }
    } catch (e) {
      debugPrint('CallService: Failed to parse event: $e');
    }
  }

  /// End the call and clean up connections
  void endCall() {
    debugPrint('CallService: Ending call $_sessionId');
    _eventsChannel?.sink.close();
    _eventsChannel = null;
    _isConnected = false;
    _sessionId = null;
  }

  void dispose() {
    endCall();
    _captionController.close();
    _statusController.close();
    _errorController.close();
    _audioController.close();
  }
}
