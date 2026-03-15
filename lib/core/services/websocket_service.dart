import 'dart:convert';
import 'dart:typed_data';
import 'package:flutter/foundation.dart' show debugPrint;

/// Configuration for the Signify backend
class SignifyConfig {
  // ── Compile-time defaults (overridable at build time via --dart-define) ──
  static const bool _defaultUseNgrok = bool.fromEnvironment(
    'SIGNIFY_USE_NGROK',
    defaultValue: false,
  );
  static const String _defaultNgrokHost = String.fromEnvironment(
    'SIGNIFY_NGROK_HOST',
    defaultValue: 'impertinently-uncommingled-aimee.ngrok-free.dev',
  );
  static const int _defaultNgrokPort = int.fromEnvironment(
    'SIGNIFY_NGROK_PORT',
    defaultValue: 443,
  );
  static const String _defaultLocalIP = String.fromEnvironment(
    'SIGNIFY_LOCAL_HOST',
    defaultValue: '10.213.158.247',
  );
  static const int _defaultLocalPort = int.fromEnvironment(
    'SIGNIFY_LOCAL_PORT',
    defaultValue: 8080,
  );

  // ── Runtime overrides (set from SettingsNotifier) ────────────────────────
  static bool? _runtimeUseNgrok;
  static String? _runtimeNgrokHost;
  static String? _runtimeLocalIP;

  /// Call this whenever backend settings change at runtime.
  static void applyRuntimeSettings({
    required bool useNgrok,
    required String ngrokHost,
    required String localIP,
  }) {
    _runtimeUseNgrok = useNgrok;
    _runtimeNgrokHost = ngrokHost;
    _runtimeLocalIP = localIP.trim().isEmpty ? null : localIP.trim();
  }

  // ── Effective values ─────────────────────────────────────────────────────
  static bool get useNgrok => _runtimeUseNgrok ?? _defaultUseNgrok;
  static String get ngrokHost => _runtimeNgrokHost ?? _defaultNgrokHost;
  static int get ngrokPort => _defaultNgrokPort;
  static String get localIP => _runtimeLocalIP ?? _defaultLocalIP;
  static int get localPort => _defaultLocalPort;

  static String get serverIP => useNgrok ? ngrokHost : localIP;
  static int get serverPort => useNgrok ? ngrokPort : localPort;

  // For ngrok (port 443), omit the port — it's the default for wss/https and
  // including it causes Dart's Uri to serialize a trailing '#' fragment which
  // breaks the WebSocket upgrade handshake.
  static String get rootUrl =>
      useNgrok ? 'wss://$serverIP' : 'ws://$serverIP:$serverPort';
  static String get apiBaseUrl =>
      useNgrok ? 'https://$serverIP' : 'http://$serverIP:$serverPort';

  static String get wsUrl => '$rootUrl/ws';
  static String get callWsUrl => '$rootUrl/call';
  static String get globalWsUrl => '$rootUrl/ws/app/global';
  static String get healthUrl => '$apiBaseUrl/health';
}

/// Message types from the backend
class BackendMessage {
  final String type;
  final String? text;
  final Uint8List? audioData;

  BackendMessage({required this.type, this.text, this.audioData});

  factory BackendMessage.fromJson(Map<String, dynamic> json) {
    Uint8List? audioData;
    if (json['audio'] != null) {
      try {
        audioData = base64Decode(json['audio'] as String);
      } catch (e) {
        debugPrint('Error decoding base64 audio: $e');
      }
    }

    return BackendMessage(
      type: json['type'] ?? 'text',
      text: json['text'] as String? ?? json['data'] as String?,
      audioData: audioData,
    );
  }
}

// P17 fix: SignifyWebSocketService and its `signifyService` singleton were dead
// code (never referenced outside this file). Removed to reduce bundle size.
