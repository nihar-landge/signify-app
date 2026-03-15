import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:record/record.dart';
import 'package:web_socket_channel/io.dart';

import 'websocket_service.dart';

/// Captures mic audio (PCM16 @ 16 kHz, mono) and streams it as binary
/// WebSocket frames to `wss://host/ws/app/audio?sessionId=`.
///
/// Used in test mode to simulate the hearing caller's voice so that
/// the Deepgram STT pipeline on the backend can produce stt_partial /
/// stt_final captions without a real Exotel/Twilio call.
class MicStreamClient {
  MicStreamClient({required this.sessionId, this.sttLanguage = 'en-US'});

  final String sessionId;
  final String sttLanguage;

  final AudioRecorder _recorder = AudioRecorder();
  IOWebSocketChannel? _channel;
  StreamSubscription<Uint8List>? _pcmSub;
  bool _active = false;

  bool get isStreaming => _active;

  /// Open the WS connection and start streaming PCM16 @ 16 kHz audio.
  Future<void> startStreaming() async {
    if (_active) return;

    final wsUrl =
        '${SignifyConfig.rootUrl}/ws/app/audio?sessionId=$sessionId&language=$sttLanguage';
    debugPrint('MicStreamClient: connecting $wsUrl');

    try {
      _channel = IOWebSocketChannel.connect(
        Uri.parse(wsUrl).removeFragment(),
      );
      await _channel!.ready.timeout(const Duration(seconds: 5));
    } catch (e) {
      debugPrint('MicStreamClient: WS connect failed: $e');
      _channel = null;
      return;
    }

    // Start recording as a raw PCM16 stream
    final stream = await _recorder.startStream(
      const RecordConfig(
        encoder: AudioEncoder.pcm16bits,
        sampleRate: 16000,
        numChannels: 1,
      ),
    );

    _active = true;
    debugPrint('MicStreamClient: streaming started');

    _pcmSub = stream.listen(
      (chunk) {
        if (!_active) return;
        try {
          _channel?.sink.add(chunk);
        } catch (e) {
          debugPrint('MicStreamClient: send error: $e');
        }
      },
      onError: (e) {
        debugPrint('MicStreamClient: stream error: $e');
        stopStreaming();
      },
      onDone: () {
        debugPrint('MicStreamClient: audio stream done');
        stopStreaming();
      },
    );
  }

  /// Stop recording and close the WebSocket.
  Future<void> stopStreaming() async {
    if (!_active) return;
    _active = false;

    await _pcmSub?.cancel();
    _pcmSub = null;

    try {
      await _recorder.stop();
    } catch (_) {}

    try {
      await _channel?.sink.close();
    } catch (_) {}
    _channel = null;

    debugPrint('MicStreamClient: streaming stopped');
  }

  Future<void> dispose() async {
    await stopStreaming();
    _recorder.dispose();
  }
}
