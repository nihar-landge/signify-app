/// App-wide Riverpod providers.
/// All providers live here so they can be easily imported across features.

library;

import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

import '../services/websocket_service.dart';
import '../services/global_websocket_service.dart';
import '../services/landmark_websocket_client.dart';
import '../services/call_service.dart';
import '../services/preferences_service.dart';

// ─── Shared Preferences ───────────────────────────────────────────────────────

final sharedPreferencesProvider = Provider<SharedPreferences>((ref) {
  throw UnimplementedError('Override in ProviderScope');
});

// ─── Preferences Service ──────────────────────────────────────────────────────

final preferencesServiceProvider = Provider<PreferencesService>((ref) {
  throw UnimplementedError('Override in ProviderScope');
});

// ─── Settings ─────────────────────────────────────────────────────────────────

class AppSettings {
  final String language; // "ASL" | "ISL"
  final String
      sttLanguage; // BCP-47 spoken language for Deepgram, e.g. "en-US", "hi"
  final bool voiceOutput;
  final ThemeMode themeMode;
  final double captionFontSize;
  final String ngrokHost;
  final bool useNgrok;

  const AppSettings({
    this.language = 'ASL',
    this.sttLanguage = 'en-US',
    this.voiceOutput = true,
    this.themeMode = ThemeMode.light,
    this.captionFontSize = 18,
    this.ngrokHost = 'impertinently-uncommingled-aimee.ngrok-free.dev',
    this.useNgrok = false,
  });

  AppSettings copyWith({
    String? language,
    String? sttLanguage,
    bool? voiceOutput,
    ThemeMode? themeMode,
    double? captionFontSize,
    String? ngrokHost,
    bool? useNgrok,
  }) {
    return AppSettings(
      language: language ?? this.language,
      sttLanguage: sttLanguage ?? this.sttLanguage,
      voiceOutput: voiceOutput ?? this.voiceOutput,
      themeMode: themeMode ?? this.themeMode,
      captionFontSize: captionFontSize ?? this.captionFontSize,
      ngrokHost: ngrokHost ?? this.ngrokHost,
      useNgrok: useNgrok ?? this.useNgrok,
    );
  }
}

class SettingsNotifier extends Notifier<AppSettings> {
  @override
  AppSettings build() {
    final prefs = ref.read(sharedPreferencesProvider);
    // Fall back to compile-time constants (--dart-define) when the user hasn't
    // explicitly saved a preference yet (null = not set).
    final useNgrok = prefs.getBool('useNgrok') ?? SignifyConfig.useNgrok;
    final ngrokHost = prefs.getString('ngrokHost') ?? SignifyConfig.ngrokHost;
    final settings = AppSettings(
      language: prefs.getString('language') ?? 'ASL',
      sttLanguage: prefs.getString('sttLanguage') ?? 'en-US',
      voiceOutput: prefs.getBool('voiceOutput') ?? true,
      themeMode: ThemeMode.values[prefs.getInt('themeMode') ?? 0],
      captionFontSize: prefs.getDouble('captionFontSize') ?? 18,
      ngrokHost: ngrokHost,
      useNgrok: useNgrok,
    );
    // Apply persisted backend config to SignifyConfig at startup.
    SignifyConfig.applyRuntimeSettings(
      useNgrok: settings.useNgrok,
      ngrokHost: settings.ngrokHost,
    );
    return settings;
  }

  Future<void> setLanguage(String lang) async {
    state = state.copyWith(language: lang);
    await ref.read(sharedPreferencesProvider).setString('language', lang);
  }

  Future<void> setSttLanguage(String lang) async {
    state = state.copyWith(sttLanguage: lang);
    await ref.read(sharedPreferencesProvider).setString('sttLanguage', lang);
  }

  Future<void> setVoiceOutput(bool v) async {
    state = state.copyWith(voiceOutput: v);
    await ref.read(sharedPreferencesProvider).setBool('voiceOutput', v);
  }

  Future<void> setThemeMode(ThemeMode mode) async {
    state = state.copyWith(themeMode: mode);
    await ref.read(sharedPreferencesProvider).setInt('themeMode', mode.index);
  }

  Future<void> setCaptionFontSize(double size) async {
    state = state.copyWith(captionFontSize: size);
    await ref
        .read(sharedPreferencesProvider)
        .setDouble('captionFontSize', size);
  }

  Future<void> setBackendConfig({
    required String ngrokHost,
    required bool useNgrok,
  }) async {
    state = state.copyWith(ngrokHost: ngrokHost, useNgrok: useNgrok);
    await ref.read(sharedPreferencesProvider).setString('ngrokHost', ngrokHost);
    await ref.read(sharedPreferencesProvider).setBool('useNgrok', useNgrok);
    // Apply immediately so all new connections use the updated config.
    SignifyConfig.applyRuntimeSettings(
      useNgrok: useNgrok,
      ngrokHost: ngrokHost,
    );
  }
}

final settingsProvider = NotifierProvider<SettingsNotifier, AppSettings>(
  SettingsNotifier.new,
);

// Convenience providers
final themeModeProvider = Provider<ThemeMode>(
  (ref) => ref.watch(settingsProvider).themeMode,
);
final selectedLanguageProvider = Provider<String>(
  (ref) => ref.watch(settingsProvider).language,
);
final sttLanguageProvider = Provider<String>(
  (ref) => ref.watch(settingsProvider).sttLanguage,
);

// ─── Global WebSocket provider ─────────────────────────────────────────────────

final globalWsServiceProvider = Provider<GlobalWebSocketService>((ref) {
  return globalWsService;
});

// Stream of incoming call events
final incomingCallStreamProvider = StreamProvider<Map<String, dynamic>>((ref) {
  final svc = ref.watch(globalWsServiceProvider);
  return svc.incomingCallStream;
});

// ─── Call State ───────────────────────────────────────────────────────────────

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

class CallNotifier extends Notifier<CallState> {
  CallSessionService? _session;
  Timer? _callTimer;
  StreamSubscription<CallCaption>? _captionSub;
  StreamSubscription<String>? _statusSub;

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
    if (state.phoneNumber.isEmpty) return;
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

    _captionSub = _session!.captionStream.listen((c) {
      final caption = Caption(
        text: c.text,
        isFinal: c.isFinal,
        isFromSigner: c.isFromSigner,
        timestamp: c.timestamp,
      );
      state = state.copyWith(captions: [...state.captions, caption]);
    });

    _statusSub = _session!.statusStream.listen((status) {
      state = state.copyWith(callStatus: status);
    });
  }

  void _startTimer() {
    _callTimer?.cancel();
    _callTimer = Timer.periodic(const Duration(seconds: 1), (_) {
      state = state.copyWith(callSeconds: state.callSeconds + 1);
    });
  }

  Future<void> endCall() async {
    _callTimer?.cancel();
    _captionSub?.cancel();
    _statusSub?.cancel();
    _session?.endCall();
    _session = null;
    state = const CallState();
  }

  void toggleMute() => state = state.copyWith(muted: !state.muted);
  void toggleSpeaker() => state = state.copyWith(speakerOn: !state.speakerOn);

  /// Stream of TTS audio bytes from the backend (sign_final events).
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

// ─── Landmark WebSocket client provider ──────────────────────────────────────

// Holds the active LandmarkWebSocketClient for the camera screen.
// Managed manually (created/destroyed by CameraScreen).
final landmarkClientProvider =
    StateProvider<LandmarkWebSocketClient?>((ref) => null);

// ─── History Provider ─────────────────────────────────────────────────────────

class ConversationSummary {
  final String id;
  final String title;
  final String language;
  final DateTime date;
  final int durationSeconds;
  final String preview;

  ConversationSummary({
    required this.id,
    required this.title,
    required this.language,
    required this.date,
    required this.durationSeconds,
    required this.preview,
  });

  factory ConversationSummary.fromJson(Map<String, dynamic> json) {
    return ConversationSummary(
      id: json['id'] as String? ?? json['conversationId'] as String? ?? '',
      title: json['title'] as String? ?? 'Session',
      language: json['language'] as String? ?? 'ASL',
      date: json['createdAt'] != null
          ? DateTime.tryParse(json['createdAt'] as String) ?? DateTime.now()
          : DateTime.now(),
      durationSeconds: json['durationSeconds'] as int? ?? 0,
      preview: json['preview'] as String? ?? '',
    );
  }
}

class ConversationMessage {
  final String text;
  final bool isFromSigner;
  final DateTime timestamp;

  ConversationMessage({
    required this.text,
    required this.isFromSigner,
    required this.timestamp,
  });

  factory ConversationMessage.fromJson(Map<String, dynamic> json) {
    return ConversationMessage(
      text: json['text'] as String? ?? '',
      isFromSigner: (json['sender'] as String?) == 'signer',
      timestamp: json['timestamp'] != null
          ? DateTime.tryParse(json['timestamp'] as String) ?? DateTime.now()
          : DateTime.now(),
    );
  }
}

class HistoryState {
  final List<ConversationSummary> conversations;
  final Map<String, List<ConversationMessage>> details;
  final bool loading;
  final String query;
  final String? error;

  const HistoryState({
    this.conversations = const [],
    this.details = const {},
    this.loading = false,
    this.query = '',
    this.error,
  });

  HistoryState copyWith({
    List<ConversationSummary>? conversations,
    Map<String, List<ConversationMessage>>? details,
    bool? loading,
    String? query,
    String? error,
  }) {
    return HistoryState(
      conversations: conversations ?? this.conversations,
      details: details ?? this.details,
      loading: loading ?? this.loading,
      query: query ?? this.query,
      error: error,
    );
  }
}

class HistoryNotifier extends Notifier<HistoryState> {
  @override
  HistoryState build() {
    // Schedule _load() after build() returns so that `state` is initialized
    // before _load() tries to read/write it.
    Future.microtask(_load);
    return const HistoryState(loading: true);
  }

  String get _userId {
    try {
      return ref.read(preferencesServiceProvider).userId;
    } catch (_) {
      return 'user_default';
    }
  }

  Future<void> _load({String query = ''}) async {
    state = state.copyWith(loading: true, error: null);
    try {
      final url = '${SignifyConfig.apiBaseUrl}/api/v1/conversations/$_userId';
      final response =
          await http.get(Uri.parse(url)).timeout(const Duration(seconds: 10));

      if (response.statusCode == 200) {
        final List<dynamic> raw = jsonDecode(response.body) as List<dynamic>;
        final convs = raw
            .map((e) => ConversationSummary.fromJson(e as Map<String, dynamic>))
            .toList();

        final filtered = query.isEmpty
            ? convs
            : convs
                .where((c) =>
                    c.title.toLowerCase().contains(query.toLowerCase()) ||
                    c.preview.toLowerCase().contains(query.toLowerCase()))
                .toList();

        state = state.copyWith(
          conversations: filtered,
          loading: false,
          query: query,
        );
      } else {
        state = state.copyWith(
          loading: false,
          error: 'Server error: ${response.statusCode}',
        );
      }
    } catch (e) {
      state = state.copyWith(
        loading: false,
        error: 'Failed to load history: $e',
      );
    }
  }

  Future<void> search(String q) => _load(query: q);
  Future<void> refresh() => _load(query: state.query);

  Future<List<ConversationMessage>> getDetail(String conversationId) async {
    if (state.details.containsKey(conversationId)) {
      return state.details[conversationId]!;
    }

    try {
      final url =
          '${SignifyConfig.apiBaseUrl}/api/v1/conversations/$_userId/$conversationId';
      final response =
          await http.get(Uri.parse(url)).timeout(const Duration(seconds: 10));

      if (response.statusCode == 200) {
        final body = jsonDecode(response.body);
        final List<dynamic> msgs = body is List
            ? body
            : (body as Map<String, dynamic>)['messages'] as List<dynamic>? ??
                [];

        final messages = msgs
            .map((e) => ConversationMessage.fromJson(e as Map<String, dynamic>))
            .toList();

        state = state.copyWith(
          details: {...state.details, conversationId: messages},
        );
        return messages;
      }
    } catch (e) {
      debugPrint('HistoryNotifier: Failed to load detail: $e');
    }
    return [];
  }
}

final historyProvider = NotifierProvider<HistoryNotifier, HistoryState>(
  HistoryNotifier.new,
);
