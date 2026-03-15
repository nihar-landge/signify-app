/// App-wide Riverpod providers.
///
/// Infrastructure providers live here. Domain-specific providers are in their
/// own files and re-exported below so all existing import sites keep working
/// with a single import.

library;

// ─── Domain providers (re-exported for backward-compatible imports) ───────────
export 'settings_provider.dart';
export 'call_provider.dart';
export 'history_provider.dart';

import 'package:camera/camera.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../services/global_websocket_service.dart';
import '../services/landmark_websocket_client.dart';
import '../services/preferences_service.dart';

// ─── Shared Preferences ───────────────────────────────────────────────────────

final sharedPreferencesProvider = Provider<SharedPreferences>((ref) {
  throw UnimplementedError('Override in ProviderScope');
});

// ─── Available Cameras (P8: resolved once, shared across screens) ─────────────

final availableCamerasProvider =
    FutureProvider<List<CameraDescription>>((ref) async {
  return availableCameras();
});

// ─── Preferences Service ──────────────────────────────────────────────────────

final preferencesServiceProvider = Provider<PreferencesService>((ref) {
  throw UnimplementedError('Override in ProviderScope');
});

// ─── Global WebSocket provider ─────────────────────────────────────────────────

final globalWsServiceProvider = Provider<GlobalWebSocketService>((ref) {
  return globalWsService;
});

// Stream of incoming call events
final incomingCallStreamProvider = StreamProvider<Map<String, dynamic>>((ref) {
  final svc = ref.watch(globalWsServiceProvider);
  return svc.incomingCallStream;
});

// ─── Landmark WebSocket client provider ──────────────────────────────────────

// Holds the active LandmarkWebSocketClient for the camera screen.
// Managed manually (created/destroyed by CameraScreen).
final landmarkClientProvider =
    StateProvider<LandmarkWebSocketClient?>((ref) => null);
