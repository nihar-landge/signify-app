import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
import 'websocket_service.dart';

/// Service to handle global events (like incoming calls) across the app
class GlobalWebSocketService {
  WebSocketChannel? _channel;
  bool _isConnected = false;
  Timer? _reconnectTimer;

  // Stream for incoming call events
  final _incomingCallController =
      StreamController<Map<String, dynamic>>.broadcast();
  Stream<Map<String, dynamic>> get incomingCallStream =>
      _incomingCallController.stream;

  bool get isConnected => _isConnected;

  /// Start the global listener
  void init() {
    _connect();
  }

  void _connect() async {
    if (_isConnected) return;

    final wsUrl = SignifyConfig.globalWsUrl;
    debugPrint('Connecting to Global WebSocket: $wsUrl');

    try {
      _channel = WebSocketChannel.connect(Uri.parse(wsUrl).removeFragment());

      _channel!.stream.listen(
        _handleMessage,
        onError: _handleError,
        onDone: _handleDone,
      );

      await _channel!.ready;
      _isConnected = true;
      debugPrint('Global WebSocket connected');

      _reconnectTimer?.cancel();
      _reconnectTimer = null;
    } catch (e) {
      debugPrint('Global WebSocket connection failed: $e');
      _scheduleReconnect();
    }
  }

  void _handleMessage(dynamic message) {
    if (message is String) {
      try {
        final data = jsonDecode(message) as Map<String, dynamic>;
        if (data['type'] == 'incoming_call') {
          debugPrint('INCOMING CALL SIGNAL RECEIVED: $data');
          _incomingCallController.add(data);
        }
      } catch (e) {
        debugPrint('Error parsing global message: $e');
      }
    }
  }

  void _handleError(dynamic error) {
    debugPrint('Global WebSocket error: $error');
    _isConnected = false;
    _scheduleReconnect();
  }

  void _handleDone() {
    debugPrint('Global WebSocket connection closed');
    _isConnected = false;
    _scheduleReconnect();
  }

  void _scheduleReconnect() {
    if (_reconnectTimer != null) return;
    debugPrint('Scheduling global WS reconnect in 5 seconds...');
    _reconnectTimer = Timer(const Duration(seconds: 5), () {
      _reconnectTimer = null;
      _connect();
    });
  }

  void dispose() {
    _reconnectTimer?.cancel();
    _channel?.sink.close();
    _incomingCallController.close();
  }
}

/// Singleton for easy app-wide access
final globalWsService = GlobalWebSocketService();
