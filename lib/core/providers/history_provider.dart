// Conversation history state and notifier.

import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart' show debugPrint;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;

import '../services/websocket_service.dart';
import 'providers.dart' show preferencesServiceProvider;

// ─── Models ───────────────────────────────────────────────────────────────────

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

// ─── HistoryState ─────────────────────────────────────────────────────────────

class HistoryState {
  final List<ConversationSummary> conversations;
  final List<ConversationSummary> allConversations; // full unfiltered list
  final Map<String, List<ConversationMessage>> details;
  final bool loading;
  final String query;
  final String? error;

  const HistoryState({
    this.conversations = const [],
    this.allConversations = const [],
    this.details = const {},
    this.loading = false,
    this.query = '',
    this.error,
  });

  HistoryState copyWith({
    List<ConversationSummary>? conversations,
    List<ConversationSummary>? allConversations,
    Map<String, List<ConversationMessage>>? details,
    bool? loading,
    String? query,
    String? error,
  }) {
    return HistoryState(
      conversations: conversations ?? this.conversations,
      allConversations: allConversations ?? this.allConversations,
      details: details ?? this.details,
      loading: loading ?? this.loading,
      query: query ?? this.query,
      error: error,
    );
  }
}

// ─── HistoryNotifier ──────────────────────────────────────────────────────────

class HistoryNotifier extends Notifier<HistoryState> {
  Timer? _searchDebounce;

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
        final body = jsonDecode(response.body);
        final List<dynamic> raw = body is List
            ? body
            : (body as Map<String, dynamic>)['conversations']
                    as List<dynamic>? ??
                [];
        final convs = raw
            .map((e) => ConversationSummary.fromJson(e as Map<String, dynamic>))
            .toList();

        // P6 fix: store full list once; filtering is done client-side.
        final filtered = _applyQuery(convs, query);
        state = state.copyWith(
          allConversations: convs,
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

  List<ConversationSummary> _applyQuery(
      List<ConversationSummary> all, String q) {
    if (q.isEmpty) return all;
    final lower = q.toLowerCase();
    return all
        .where((c) =>
            c.title.toLowerCase().contains(lower) ||
            c.preview.toLowerCase().contains(lower))
        .toList();
  }

  /// P5 fix: debounced search that filters the already-loaded list client-side
  /// instead of making a new HTTP request on every keystroke.
  void search(String q) {
    _searchDebounce?.cancel();
    _searchDebounce = Timer(const Duration(milliseconds: 300), () {
      state = state.copyWith(
        conversations: _applyQuery(state.allConversations, q),
        query: q,
      );
    });
  }

  Future<void> refresh() => _load(query: state.query);

  Future<List<ConversationMessage>> getDetail(String conversationId) async {
    // P18 fix: early-return on cache hit without touching state, so no
    // spurious rebuild is triggered.
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
