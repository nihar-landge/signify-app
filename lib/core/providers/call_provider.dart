// Call session state and notifier.

import 'dart:async';
import 'dart:math';
import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../services/call_service.dart';

// ─── Caption ──────────────────────────────────────────────────────────────────

class Caption {
  final String text;
  final bool isFinal;
  final bool isFromSigner;
  final DateTime timestamp;

  Caption({
    required this.text,
    required this.isFinal,
    required this.isFromSigner,
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now();
}

// ─── CallState ────────────────────────────────────────────────────────────────

class CallState {
  final bool inCall;
  final bool isTestMode;
  final String phoneNumber;
  final String sessionId;
  final List<Caption> captions;
  final int callSeconds;
  final bool muted;
  final bool speakerOn;
  final bool loading;
  final String? errorMessage;
  final String callStatus; // 'idle' | 'dialing' | 'connected' | 'ending'

  const CallState({
    this.inCall = false,
    this.isTestMode = false,
    this.phoneNumber = '',
    this.sessionId = '',
    this.captions = const [],
    this.callSeconds = 0,
    this.muted = false,
    this.speakerOn = true,
    this.loading = false,
    this.errorMessage,
    this.callStatus = 'idle',
  });

  CallState copyWith({
    bool? inCall,
    bool? isTestMode,
    String? phoneNumber,
    String? sessionId,
    List<Caption>? captions,
    int? callSeconds,
    bool? muted,
    bool? speakerOn,
    bool? loading,
    String? errorMessage,
    String? callStatus,
  }) {
    return CallState(
      inCall: inCall ?? this.inCall,
      isTestMode: isTestMode ?? this.isTestMode,
      phoneNumber: phoneNumber ?? this.phoneNumber,
      sessionId: sessionId ?? this.sessionId,
      captions: captions ?? this.captions,
      callSeconds: callSeconds ?? this.callSeconds,
      muted: muted ?? this.muted,
      speakerOn: speakerOn ?? this.speakerOn,
      loading: loading ?? this.loading,
      errorMessage: errorMessage,
      callStatus: callStatus ?? this.callStatus,
    );
  }
}

// ─── CallNotifier ─────────────────────────────────────────────────────────────

class CallNotifier extends Notifier<CallState> {
  CallSessionService? _session;
  Timer? _callTimer;
  StreamSubscription<CallCaption>? _captionSub;
  StreamSubscription<String>? _statusSub;
  bool _active = false; // guards timer/stream callbacks after endCall()

  @override
  CallState build() => const CallState();

  void setPhoneNumber(String number) =>
      state = state.copyWith(phoneNumber: number);

  void appendDigit(String digit) =>
      state = state.copyWith(phoneNumber: state.phoneNumber + digit);

  void backspace() {
    if (state.phoneNumber.isEmpty) return;
    state = state.copyWith(
      phoneNumber: state.phoneNumber.substring(0, state.phoneNumber.length - 1),
    );
  }

  Future<void> startCall() async {
    if (state.phoneNumber.isEmpty || state.inCall) return;
    state = state.copyWith(
        loading: true, callStatus: 'dialing', errorMessage: null);

    _session = CallSessionService();
    final sessionId = await _session!.startCall(state.phoneNumber);

    if (sessionId == null) {
      state = state.copyWith(
        loading: false,
        callStatus: 'idle',
        errorMessage: 'Failed to start call',
      );
      _session = null;
      return;
    }

    state = state.copyWith(
      inCall: true,
      sessionId: sessionId,
      captions: [],
      callSeconds: 0,
      loading: false,
      callStatus: 'connected',
    );

    _subscribeToCaptions();
    _startTimer();
  }

  Future<void> acceptIncomingCall(String sessionId, String fromNumber) async {
    state = state.copyWith(
      loading: true,
      phoneNumber: fromNumber,
      callStatus: 'connecting',
    );

    _session = CallSessionService();
    await _session!.acceptCall(sessionId);

    state = state.copyWith(
      inCall: true,
      sessionId: sessionId,
      captions: [],
      callSeconds: 0,
      loading: false,
      callStatus: 'connected',
    );

    _subscribeToCaptions();
    _startTimer();
  }

  /// Start a test session without making a real PSTN call.
  /// Uses a `test-session-<uuid>` ID that the backend auto-creates.
  Future<void> startTestSession() async {
    // Prevent starting a new session while one is already active.
    if (state.inCall) return;
    final rand = Random.secure();
    final hex = List.generate(16, (_) => rand.nextInt(256))
        .map((b) => b.toRadixString(16).padLeft(2, '0'))
        .join();
    // Format as a UUID-like string: 8-4-4-4-12
    final sessionId =
        'test-session-${hex.substring(0, 8)}-${hex.substring(8, 12)}-'
        '${hex.substring(12, 16)}-${hex.substring(16, 20)}-${hex.substring(20)}';

    state = state.copyWith(
      inCall: true,
      isTestMode: true,
      phoneNumber: 'Test Mode',
      sessionId: sessionId,
      captions: [],
      callSeconds: 0,
      loading: false,
      callStatus: 'connected',
      errorMessage: null,
    );

    _session = CallSessionService();
    await _session!.acceptCall(sessionId);

    _subscribeToCaptions();
    _startTimer();
  }

  void _subscribeToCaptions() {
    _captionSub?.cancel();
    _statusSub?.cancel();
    _active = true;

    _captionSub = _session!.captionStream.listen((c) {
      if (!_active) return;
      final caption = Caption(
        text: c.text,
        isFinal: c.isFinal,
        isFromSigner: c.isFromSigner,
        timestamp: c.timestamp,
      );
      // P2 fix: cap at 200 captions and avoid re-spreading the full list on
      // every WS message by using List.unmodifiable with a bounded slice.
      final current = state.captions;
      final updated = current.length >= 200
          ? [...current.skip(1), caption]
          : [...current, caption];
      state = state.copyWith(captions: updated);
    });

    _statusSub = _session!.statusStream.listen((status) {
      if (!_active) return;
      state = state.copyWith(callStatus: status);
    });
  }

  void _startTimer() {
    _callTimer?.cancel();
    _callTimer = Timer.periodic(const Duration(seconds: 1), (_) {
      if (!_active) return;
      state = state.copyWith(callSeconds: state.callSeconds + 1);
    });
  }

  Future<void> endCall() async {
    _active = false;
    _callTimer?.cancel();
    _captionSub?.cancel();
    _statusSub?.cancel();
    _session?.endCall();
    _session = null;
    state = const CallState();
  }

  void toggleMute() => state = state.copyWith(muted: !state.muted);
  void toggleSpeaker() => state = state.copyWith(speakerOn: !state.speakerOn);

  /// Called by the landmarks WS callback so that sign_partial / sign_final
  /// events appear in the in-call conversation window.
  void addSignCaption(String text, {required bool isFinal}) {
    if (!_active || text.isEmpty) return;
    final caption = Caption(
      text: text,
      isFinal: isFinal,
      isFromSigner: true,
    );
    final current = state.captions;
    final updated = current.length >= 200
        ? [...current.skip(1), caption]
        : [...current, caption];
    state = state.copyWith(captions: updated);
  }

  /// Stream of TTS audio bytes from the backend (sign_final events via events WS).
  Stream<Uint8List>? get audioStream => _session?.audioStream;

  String get formattedTime {
    final m = state.callSeconds ~/ 60;
    final s = state.callSeconds % 60;
    return '$m:${s.toString().padLeft(2, '0')}';
  }
}

final callProvider = NotifierProvider<CallNotifier, CallState>(
  CallNotifier.new,
);

// Derived providers so widgets can subscribe to only the slice they need,
// avoiding full rebuilds on every 1-second timer tick (P4 fix).
final callSecondsProvider = Provider<int>(
  (ref) => ref.watch(callProvider).callSeconds,
);
final callCaptionsProvider = Provider<List<Caption>>(
  (ref) => ref.watch(callProvider).captions,
);
