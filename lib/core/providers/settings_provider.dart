// Settings state and notifier.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../services/websocket_service.dart';
import 'providers.dart' show sharedPreferencesProvider;

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
  final String localIP; // LAN IP of the backend when not using ngrok

  const AppSettings({
    this.language = 'ASL',
    this.sttLanguage = 'en-US',
    this.voiceOutput = true,
    this.themeMode = ThemeMode.light,
    this.captionFontSize = 18,
    this.ngrokHost = 'impertinently-uncommingled-aimee.ngrok-free.dev',
    this.useNgrok = false,
    this.localIP = '10.213.158.247',
  });

  AppSettings copyWith({
    String? language,
    String? sttLanguage,
    bool? voiceOutput,
    ThemeMode? themeMode,
    double? captionFontSize,
    String? ngrokHost,
    bool? useNgrok,
    String? localIP,
  }) {
    return AppSettings(
      language: language ?? this.language,
      sttLanguage: sttLanguage ?? this.sttLanguage,
      voiceOutput: voiceOutput ?? this.voiceOutput,
      themeMode: themeMode ?? this.themeMode,
      captionFontSize: captionFontSize ?? this.captionFontSize,
      ngrokHost: ngrokHost ?? this.ngrokHost,
      useNgrok: useNgrok ?? this.useNgrok,
      localIP: localIP ?? this.localIP,
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
    final localIP = prefs.getString('localIP') ?? SignifyConfig.localIP;
    final settings = AppSettings(
      language: prefs.getString('language') ?? 'ASL',
      sttLanguage: prefs.getString('sttLanguage') ?? 'en-US',
      voiceOutput: prefs.getBool('voiceOutput') ?? true,
      themeMode: ThemeMode.values[prefs.getInt('themeMode') ?? 0],
      captionFontSize: prefs.getDouble('captionFontSize') ?? 18,
      ngrokHost: ngrokHost,
      useNgrok: useNgrok,
      localIP: localIP,
    );
    // Apply persisted backend config to SignifyConfig at startup.
    SignifyConfig.applyRuntimeSettings(
      useNgrok: settings.useNgrok,
      ngrokHost: settings.ngrokHost,
      localIP: settings.localIP,
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
    required String localIP,
  }) async {
    state = state.copyWith(
        ngrokHost: ngrokHost, useNgrok: useNgrok, localIP: localIP);
    await ref.read(sharedPreferencesProvider).setString('ngrokHost', ngrokHost);
    await ref.read(sharedPreferencesProvider).setBool('useNgrok', useNgrok);
    await ref.read(sharedPreferencesProvider).setString('localIP', localIP);
    // Apply immediately so all new connections use the updated config.
    SignifyConfig.applyRuntimeSettings(
      useNgrok: useNgrok,
      ngrokHost: ngrokHost,
      localIP: localIP,
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
