import 'dart:async';

import 'package:audioplayers/audioplayers.dart';
import 'package:camera/camera.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:permission_handler/permission_handler.dart';

import '../../core/providers/providers.dart';
import '../../core/services/landmark_extractor.dart';
import '../../core/services/landmark_websocket_client.dart';
import '../../core/services/mic_stream_client.dart';
import '../../core/services/websocket_service.dart';
import '../../core/theme/app_theme.dart';

// Available cameras resolved once — shared with camera_screen.dart via
// availableCamerasProvider defined in providers.dart.
// Keep a local alias for backward compatibility within this file.
// (The real provider lives in providers.dart to avoid duplicate platform calls.)
final _callAvailableCamerasProvider = availableCamerasProvider;

class CallScreen extends ConsumerStatefulWidget {
  const CallScreen({super.key});

  @override
  ConsumerState<CallScreen> createState() => _CallScreenState();
}

class _CallScreenState extends ConsumerState<CallScreen> {
  @override
  Widget build(BuildContext context) {
    final call = ref.watch(callProvider);

    if (call.inCall) {
      return _InCallView(
        onEnd: () => ref.read(callProvider.notifier).endCall(),
      );
    }

    return const _DialPadView();
  }
}

/// P4 fix: isolated Consumer that only rebuilds on every 1-second timer tick,
/// not the entire _InCallView.
class _CallTimer extends ConsumerWidget {
  const _CallTimer();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final seconds = ref.watch(callSecondsProvider);
    final m = seconds ~/ 60;
    final s = seconds % 60;
    final formatted = '$m:${s.toString().padLeft(2, '0')}';
    return Text(
      formatted,
      style: const TextStyle(
        fontFamily: 'monospace',
        fontSize: 12,
        fontWeight: FontWeight.w600,
      ),
    );
  }
}

// ─── Dial Pad View ─────────────────────────────────────────────────────────────

class _DialPadView extends ConsumerWidget {
  const _DialPadView();

  static const _dialPad = [
    '1',
    '2',
    '3',
    '4',
    '5',
    '6',
    '7',
    '8',
    '9',
    '*',
    '0',
    '#',
  ];

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final cs = Theme.of(context).colorScheme;
    final call = ref.watch(callProvider);
    final notifier = ref.read(callProvider.notifier);

    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(20, 24, 20, 32),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 400),
        child: Column(
          children: [
            Text(
              'Make a Call',
              style: Theme.of(context).textTheme.headlineMedium,
            ).animate().fade(duration: 350.ms),

            const SizedBox(height: 24),

            // ── Number display ─────────────────────────────────────────────
            Container(
              height: 64,
              width: double.infinity,
              decoration: BoxDecoration(
                color: cs.surface,
                borderRadius: BorderRadius.circular(18),
                border: Border.all(color: cs.outline.withValues(alpha: 0.5)),
              ),
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Expanded(
                    child: Text(
                      call.phoneNumber.isEmpty
                          ? 'Enter number'
                          : call.phoneNumber,
                      style: TextStyle(
                        fontFamily: 'monospace',
                        fontSize: 22,
                        fontWeight: FontWeight.w600,
                        letterSpacing: 2,
                        color: call.phoneNumber.isEmpty
                            ? cs.onSurface.withValues(alpha: 0.35)
                            : cs.onSurface,
                      ),
                    ),
                  ),
                  if (call.phoneNumber.isNotEmpty)
                    IconButton(
                      onPressed: notifier.backspace,
                      icon: Icon(Icons.backspace_outlined,
                          size: 22, color: cs.onSurface.withValues(alpha: 0.6)),
                    ),
                ],
              ),
            ).animate(delay: 80.ms).fade(duration: 350.ms),

            const SizedBox(height: 24),

            // ── Dial pad grid ──────────────────────────────────────────────
            ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 300),
              child: GridView.count(
                crossAxisCount: 3,
                shrinkWrap: true,
                mainAxisSpacing: 12,
                crossAxisSpacing: 12,
                childAspectRatio: 1.4,
                physics: const NeverScrollableScrollPhysics(),
                children: _dialPad.map((d) => _DialKey(digit: d)).toList(),
              ),
            ).animate(delay: 120.ms).fade(duration: 350.ms),

            const SizedBox(height: 28),

            // ── Call button ────────────────────────────────────────────────
            GestureDetector(
              onTap: call.loading
                  ? null
                  : () async {
                      await ref.read(callProvider.notifier).startCall();
                    },
              child: Container(
                height: 64,
                width: 64,
                decoration: BoxDecoration(
                  color: AppColors.success,
                  shape: BoxShape.circle,
                  boxShadow: [
                    BoxShadow(
                      color: AppColors.success.withValues(alpha: 0.35),
                      blurRadius: 20,
                      offset: const Offset(0, 8),
                    ),
                  ],
                ),
                child: call.loading
                    ? const Center(
                        child: SizedBox(
                          height: 24,
                          width: 24,
                          child: CircularProgressIndicator(
                            strokeWidth: 2.5,
                            color: Colors.white,
                          ),
                        ),
                      )
                    : const Icon(Icons.phone_rounded,
                        color: Colors.white, size: 28),
              ),
            ).animate(delay: 160.ms).fade(duration: 350.ms),

            const SizedBox(height: 28),

            // ── Test Mode banner ───────────────────────────────────────────
            GestureDetector(
              onTap: () async {
                await ref.read(callProvider.notifier).startTestSession();
              },
              child: Container(
                width: double.infinity,
                padding:
                    const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
                decoration: BoxDecoration(
                  color: cs.secondaryContainer.withValues(alpha: 0.55),
                  borderRadius: BorderRadius.circular(16),
                  border: Border.all(
                    color: cs.secondary.withValues(alpha: 0.35),
                  ),
                ),
                child: Row(
                  children: [
                    Icon(Icons.science_outlined, size: 20, color: cs.secondary),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'Test Sign Recognition',
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w700,
                              color: cs.onSecondaryContainer,
                            ),
                          ),
                          const SizedBox(height: 2),
                          Text(
                            'Try the sign-to-text pipeline without a real call',
                            style: TextStyle(
                              fontSize: 11,
                              color: cs.onSecondaryContainer
                                  .withValues(alpha: 0.65),
                            ),
                          ),
                        ],
                      ),
                    ),
                    Icon(Icons.chevron_right_rounded,
                        size: 20, color: cs.secondary),
                  ],
                ),
              ),
            ).animate(delay: 200.ms).fade(duration: 350.ms),
          ],
        ),
      ),
    );
  }
}

// P9 fix: _DialKey only calls ref.read in a tap callback — no need to be a
// ConsumerWidget (which would subscribe to provider changes on every tap).
class _DialKey extends StatelessWidget {
  const _DialKey({required this.digit});
  final String digit;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return GestureDetector(
      onTap: () {
        // Use ProviderScope.containerOf to read without subscribing.
        ProviderScope.containerOf(context, listen: false)
            .read(callProvider.notifier)
            .appendDigit(digit);
      },
      child: Container(
        decoration: BoxDecoration(
          color: cs.surface,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: cs.outline.withValues(alpha: 0.4)),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.04),
              blurRadius: 4,
            ),
          ],
        ),
        child: Center(
          child: Text(
            digit,
            style: TextStyle(
              fontSize: 22,
              fontWeight: FontWeight.w700,
              color: cs.onSurface,
            ),
          ),
        ),
      ),
    );
  }
}

// ─── In-Call View ──────────────────────────────────────────────────────────────

class _InCallView extends ConsumerStatefulWidget {
  const _InCallView({required this.onEnd});
  final VoidCallback onEnd;

  @override
  ConsumerState<_InCallView> createState() => _InCallViewState();
}

class _InCallViewState extends ConsumerState<_InCallView>
    with WidgetsBindingObserver {
  final _scrollController = ScrollController();

  // Camera
  CameraController? _controller;
  bool _cameraPermissionGranted = false;
  bool _cameraInitializing = false;
  bool _cameraError = false;
  int _selectedCameraIndex = 0;

  // Landmark pipeline
  LandmarkExtractor? _extractor;
  LandmarkWebSocketClient? _wsClient;
  AudioPlayer? _audioPlayer;
  StreamSubscription<dynamic>? _audioSub;
  bool _wsConnected = false;
  // Generation counter: incremented on every _stopLandmarkPipeline() call.
  // _startLandmarkPipeline() checks this after each await to abort if a newer
  // pipeline was started (prevents ghost _wsClient instances on camera flips).
  int _pipelineGen = 0;

  // Push-to-Sign state
  bool _pushing = false;

  // Hold-to-Speak state (test mode only)
  MicStreamClient? _micClient;
  bool _speaking = false;

  // Caption count — tracked to scroll only when new captions arrive.
  int _lastCaptionCount = 0;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _requestCameraPermission();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _stopLandmarkPipeline();
    _controller?.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final controller = _controller;
    if (controller == null || !controller.value.isInitialized) return;
    if (state == AppLifecycleState.inactive) {
      controller.dispose();
    } else if (state == AppLifecycleState.resumed) {
      _initCamera(controller.description);
    }
  }

  // ── Camera setup ────────────────────────────────────────────────────────────

  Future<void> _requestCameraPermission() async {
    final status = await Permission.camera.request();
    if (!mounted) return;
    setState(() => _cameraPermissionGranted = status.isGranted);
    if (status.isGranted) _pickAndInitCamera();
  }

  void _pickAndInitCamera() {
    final cameras = ref.read(_callAvailableCamerasProvider).valueOrNull;
    if (cameras == null || cameras.isEmpty) return;
    final frontIndex =
        cameras.indexWhere((c) => c.lensDirection == CameraLensDirection.front);
    _selectedCameraIndex = frontIndex >= 0 ? frontIndex : 0;
    _initCamera(cameras[_selectedCameraIndex]);
  }

  Future<void> _initCamera(CameraDescription description) async {
    debugPrint('Camera: _initCamera(${description.name}) entered');
    setState(() {
      _cameraInitializing = true;
      _cameraError = false;
    });

    // Stop the landmark pipeline (image stream + WS) before touching the camera.
    // Must happen before prev.dispose() — disposing a controller with an active
    // image stream throws a CameraException on Android.
    _stopLandmarkPipeline();

    final prev = _controller;
    if (prev != null) {
      debugPrint('Camera: disposing previous controller');
      await prev.dispose();
    }

    final controller = CameraController(
      description,
      ResolutionPreset.medium,
      enableAudio: false,
      imageFormatGroup: ImageFormatGroup.yuv420,
    );
    try {
      debugPrint('Camera: initializing new controller…');
      await controller.initialize();
      debugPrint('Camera: controller initialized');
      if (!mounted) {
        await controller.dispose();
        return;
      }
      setState(() {
        _controller = controller;
        _cameraInitializing = false;
      });
      // Start streaming landmarks as soon as camera is ready.
      _startLandmarkPipeline();
    } on CameraException catch (e) {
      debugPrint('Camera: CameraException during init: $e');
      await controller.dispose();
      if (!mounted) return;
      setState(() {
        _cameraInitializing = false;
        _cameraError = true;
      });
    }
  }

  Future<void> _flipCamera() async {
    final cameras = ref.read(_callAvailableCamerasProvider).valueOrNull;
    if (cameras == null || cameras.length < 2) return;
    _selectedCameraIndex = (_selectedCameraIndex + 1) % cameras.length;
    _stopLandmarkPipeline();
    await _initCamera(cameras[_selectedCameraIndex]);
  }

  // ── Landmark pipeline ────────────────────────────────────────────────────────

  Future<void> _startLandmarkPipeline() async {
    debugPrint('Pipeline: _startLandmarkPipeline() entered');
    final myGen = ++_pipelineGen;
    final controller = _controller;
    if (controller == null || !controller.value.isInitialized) {
      debugPrint('Pipeline: ABORT — controller null or not initialized');
      return;
    }

    final sessionId = ref.read(callProvider).sessionId;
    final language = ref.read(selectedLanguageProvider);
    debugPrint('Pipeline: sessionId="$sessionId" language="$language"');
    if (sessionId.isEmpty) {
      debugPrint('Pipeline: ABORT — sessionId is empty');
      return;
    }

    debugPrint('Pipeline: initializing LandmarkExtractor…');
    _extractor = LandmarkExtractor();
    await _extractor!.initialize();

    // Abort if a newer pipeline superseded this one while we were awaiting.
    if (_pipelineGen != myGen) {
      debugPrint('Pipeline: cancelled after initialize (gen mismatch)');
      _extractor?.dispose();
      _extractor = null;
      return;
    }
    debugPrint('Pipeline: LandmarkExtractor ready');

    _audioPlayer = AudioPlayer();

    final wsUrl = SignifyConfig.rootUrl;
    debugPrint('Pipeline: creating LandmarkWebSocketClient url=$wsUrl');
    _wsClient = LandmarkWebSocketClient(
      serverUrl: wsUrl,
      sessionId: sessionId,
      requestLanguage: language,
    );

    _wsClient!.onConnected = () {
      debugPrint('Pipeline: WS onConnected');
      if (mounted) setState(() => _wsConnected = true);
    };
    _wsClient!.onDisconnected = () {
      debugPrint('Pipeline: WS onDisconnected');
      if (mounted) setState(() => _wsConnected = false);
    };

    // Wire sign_partial / sign_final events from the landmarks WS into the
    // call provider's caption list so they appear in the conversation window.
    _wsClient!.onSignEvent = (text, isFinal) {
      debugPrint('Pipeline: onSignEvent text="$text" isFinal=$isFinal');
      if (!mounted) return;
      ref.read(callProvider.notifier).addSignCaption(text, isFinal: isFinal);
    };

    // TTS audio arrives on the landmarks WS sign_final.audio field — play it.
    _wsClient!.onAudioReceived = (Uint8List audioBytes) async {
      debugPrint('Pipeline: onAudioReceived ${audioBytes.length} bytes');
      try {
        if (ref.read(settingsProvider).voiceOutput) {
          await _audioPlayer?.play(BytesSource(audioBytes));
        }
      } catch (_) {}
    };

    // Also subscribe to TTS audio from the events WS (real call path).
    final audioStream = ref.read(callProvider.notifier).audioStream;
    if (audioStream != null) {
      _audioSub = audioStream.listen((audioBytes) async {
        try {
          if (ref.read(settingsProvider).voiceOutput) {
            await _audioPlayer?.play(BytesSource(audioBytes));
          }
        } catch (_) {}
      });
    }

    debugPrint('Pipeline: calling _wsClient.connect()…');
    await _wsClient!.connect();

    // Abort if superseded while awaiting connect().
    if (_pipelineGen != myGen) {
      debugPrint('Pipeline: cancelled after connect (gen mismatch)');
      _wsClient?.disconnect();
      _wsClient = null;
      return;
    }
    debugPrint(
        'Pipeline: connect() returned, isConnected=${_wsClient?.isConnected}');

    // In test mode, set up mic client for the Hold-to-Speak STT test pipeline.
    if (ref.read(callProvider).isTestMode) {
      final micPerm = await Permission.microphone.request();
      debugPrint('Pipeline: mic permission=${micPerm.name}');
      if (_pipelineGen != myGen) {
        debugPrint('Pipeline: cancelled after mic permission (gen mismatch)');
        return;
      }
      if (micPerm.isGranted) {
        _micClient = MicStreamClient(
          sessionId: sessionId,
          sttLanguage: ref.read(sttLanguageProvider),
        );
        debugPrint('Pipeline: MicStreamClient created');
      } else {
        debugPrint('Pipeline: WARN — microphone permission denied');
      }
    }

    debugPrint('Pipeline: starting camera image stream…');
    await controller.startImageStream((CameraImage image) {
      _extractor?.processImage(image, controller.description).then((frame) {
        if (frame == null) return;
        // Push-to-Sign: only stream landmark data while the button is held.
        // When not pushing, send a heartbeat with present=false so the backend
        // knows the session is still alive but no signing is occurring.
        final pushing = _pushing;
        final landmarks = pushing ? frame.toFullHolisticLandmarks() : null;
        _wsClient?.sendLandmarks(
          present: pushing && frame.hasData,
          landmarks: landmarks,
        );
      });
    });
    debugPrint('Pipeline: image stream started');
  }

  void _stopLandmarkPipeline() {
    // Invalidate any in-flight _startLandmarkPipeline() coroutine so it
    // abandons its work at the next await checkpoint rather than creating a
    // ghost _wsClient with a stale sessionId.
    _pipelineGen++;
    _controller?.stopImageStream().catchError((_) {});
    _wsClient?.disconnect();
    _wsClient = null;
    _extractor?.dispose();
    _extractor = null;
    _audioSub?.cancel();
    _audioSub = null;
    _audioPlayer?.dispose();
    _audioPlayer = null;
    _micClient?.dispose();
    _micClient = null;
  }

  // ── Push-to-Sign handlers ────────────────────────────────────────────────────

  void _startPushing() {
    if (!mounted) return;
    setState(() => _pushing = true);
    HapticFeedback.heavyImpact();
  }

  void _stopPushing() {
    if (!mounted) return;
    setState(() => _pushing = false);
    HapticFeedback.lightImpact();
  }

  // ── Hold-to-Speak handlers (test mode STT) ───────────────────────────────────

  void _startSpeaking() {
    if (!mounted || _micClient == null) return;
    setState(() => _speaking = true);
    HapticFeedback.heavyImpact();
    _micClient!.startStreaming();
  }

  void _stopSpeaking() {
    if (!mounted) return;
    setState(() => _speaking = false);
    HapticFeedback.lightImpact();
    _micClient?.stopStreaming();
  }

  // ── Camera preview widget ────────────────────────────────────────────────────

  Widget _buildCameraPreview(ColorScheme cs, String language) {
    if (!_cameraPermissionGranted) {
      return Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.no_photography_rounded,
              size: 32, color: cs.onSurface.withValues(alpha: 0.3)),
          const SizedBox(height: 6),
          Text('Camera permission needed',
              style: TextStyle(
                  fontSize: 11, color: cs.onSurface.withValues(alpha: 0.4))),
          const SizedBox(height: 8),
          FilledButton.tonal(
              onPressed: _requestCameraPermission, child: const Text('Grant')),
        ],
      );
    }
    if (_cameraInitializing) {
      return Center(child: CircularProgressIndicator(color: cs.primary));
    }
    if (_cameraError) {
      return Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.error_outline_rounded,
              size: 32, color: cs.onSurface.withValues(alpha: 0.3)),
          const SizedBox(height: 6),
          Text('Camera unavailable',
              style: TextStyle(
                  fontSize: 11, color: cs.onSurface.withValues(alpha: 0.4))),
        ],
      );
    }
    final controller = _controller;
    if (controller == null || !controller.value.isInitialized) {
      return Center(child: CircularProgressIndicator(color: cs.primary));
    }

    return Stack(
      fit: StackFit.expand,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(20),
          child: FittedBox(
            fit: BoxFit.cover,
            child: SizedBox(
              width: controller.value.previewSize?.height ?? 1,
              height: controller.value.previewSize?.width ?? 1,
              child: CameraPreview(controller),
            ),
          ),
        ),
        Positioned(left: 12, top: 12, child: _RecBadgeSmall()),
        Positioned(
          right: 12,
          top: 12,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
            decoration: BoxDecoration(
              color: Colors.black.withValues(alpha: 0.45),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Text(
              language,
              style: const TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w600,
                  color: Colors.white),
            ),
          ),
        ),
        Positioned(
          right: 12,
          bottom: 12,
          child: GestureDetector(
            onTap: _flipCamera,
            child: Container(
              height: 36,
              width: 36,
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.45),
                shape: BoxShape.circle,
              ),
              child: const Icon(Icons.flip_camera_ios_rounded,
                  size: 18, color: Colors.white),
            ),
          ),
        ),
        if (_wsConnected)
          Positioned(
            left: 12,
            bottom: 12,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
              decoration: BoxDecoration(
                color: AppColors.success.withValues(alpha: 0.85),
                borderRadius: BorderRadius.circular(8),
              ),
              child: const Text('● Live',
                  style: TextStyle(
                      fontSize: 9,
                      fontWeight: FontWeight.w700,
                      color: Colors.white)),
            ),
          ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    // Watch only the fields that affect the full layout (not callSeconds — that
    // is isolated in _CallTimer to prevent rebuilding the whole view every tick).
    final call = ref.watch(callProvider);
    final language = ref.watch(selectedLanguageProvider);
    // Watch captions via the scoped provider so only the caption list rebuilds
    // when a new caption arrives (not the camera preview, controls, etc.).
    final captions = ref.watch(callCaptionsProvider);

    // Auto-scroll captions only when new ones arrive (P3 fix: not every build).
    final captionCount = captions.length;
    if (captionCount != _lastCaptionCount) {
      _lastCaptionCount = captionCount;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (_scrollController.hasClients) {
          _scrollController.animateTo(
            _scrollController.position.maxScrollExtent,
            duration: const Duration(milliseconds: 300),
            curve: Curves.easeOut,
          );
        }
      });
    }

    // When cameras list resolves, pick and init camera if permission was already
    // granted (handles the race where permission resolves before the
    // FutureProvider completes).
    ref.listen(_callAvailableCamerasProvider, (_, next) {
      if (next.hasValue && _cameraPermissionGranted && _controller == null) {
        _pickAndInitCamera();
      }
    });

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
      child: Column(
        children: [
          // ── Call header ────────────────────────────────────────────────
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    call.isTestMode ? 'Test Mode' : 'In call with',
                    style: TextStyle(
                      fontSize: 11,
                      color: call.isTestMode
                          ? cs.secondary
                          : cs.onSurface.withValues(alpha: 0.5),
                      fontWeight:
                          call.isTestMode ? FontWeight.w700 : FontWeight.normal,
                    ),
                  ),
                  Text(
                    call.isTestMode
                        ? 'Sign Recognition Test'
                        : (call.phoneNumber.isEmpty
                            ? '+1 (555) 000-1234'
                            : call.phoneNumber),
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
              Row(
                children: [
                  _ConnectedBadge(),
                  const SizedBox(width: 8),
                  // P4 fix: timer display is isolated in its own Consumer so
                  // the 1-second tick only rebuilds this small widget.
                  const _CallTimer(),
                ],
              ),
            ],
          ),

          const SizedBox(height: 12),

          // ── Camera preview (live landmark streaming) ───────────────────
          Expanded(
            flex: 5,
            child: Container(
              width: double.infinity,
              decoration: BoxDecoration(
                color: cs.surface,
                borderRadius: BorderRadius.circular(20),
                border: Border.all(color: cs.outline.withValues(alpha: 0.4)),
              ),
              child: _buildCameraPreview(cs, language),
            ),
          ),

          const SizedBox(height: 10),

          // ── Live captions ──────────────────────────────────────────────
          Expanded(
            flex: 6,
            child: Container(
              decoration: BoxDecoration(
                color: cs.surface,
                borderRadius: BorderRadius.circular(20),
                border: Border.all(color: cs.outline.withValues(alpha: 0.4)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Padding(
                    padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
                    child: Text(
                      'LIVE CONVERSATION',
                      style: Theme.of(context).textTheme.labelSmall?.copyWith(
                            letterSpacing: 1.2,
                            fontWeight: FontWeight.w700,
                          ),
                    ),
                  ),
                  const Divider(height: 1),
                  Expanded(
                    child: ListView.builder(
                      controller: _scrollController,
                      padding: const EdgeInsets.all(12),
                      itemCount: captions.length,
                      itemBuilder: (context, i) =>
                          _CaptionBubble(caption: captions[i]),
                    ),
                  ),
                ],
              ),
            ),
          ),

          const SizedBox(height: 14),

          // ── Call controls: End Call + Push-to-Sign ─────────────────────
          Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              // End Call
              GestureDetector(
                onTap: widget.onEnd,
                child: Container(
                  height: 64,
                  width: 64,
                  decoration: BoxDecoration(
                    color: AppColors.destructive,
                    shape: BoxShape.circle,
                    boxShadow: [
                      BoxShadow(
                        color: AppColors.destructive.withValues(alpha: 0.35),
                        blurRadius: 20,
                        offset: const Offset(0, 6),
                      ),
                    ],
                  ),
                  child: const Icon(Icons.call_end_rounded,
                      color: Colors.white, size: 28),
                ),
              ),

              const SizedBox(width: 24),

              // Push-to-Sign
              GestureDetector(
                onTapDown: (_) => _startPushing(),
                onTapUp: (_) => _stopPushing(),
                onTapCancel: _stopPushing,
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 120),
                  height: 64,
                  width: 64,
                  decoration: BoxDecoration(
                    color: _pushing ? cs.primary : cs.surfaceContainerHighest,
                    shape: BoxShape.circle,
                    border: Border.all(
                      color: _pushing
                          ? cs.primary
                          : cs.outline.withValues(alpha: 0.5),
                      width: _pushing ? 2.5 : 1.5,
                    ),
                    boxShadow: _pushing
                        ? [
                            BoxShadow(
                              color: cs.primary.withValues(alpha: 0.35),
                              blurRadius: 20,
                              offset: const Offset(0, 6),
                            ),
                          ]
                        : null,
                  ),
                  child: Icon(
                    _pushing
                        ? Icons.sign_language_rounded
                        : Icons.sign_language_outlined,
                    color: _pushing
                        ? Colors.white
                        : cs.onSurface.withValues(alpha: 0.6),
                    size: 26,
                  ),
                ),
              ),
            ],
          ),

          // ── Push-to-Sign hint label ────────────────────────────────────
          const SizedBox(height: 6),
          Text(
            _pushing ? 'Signing...' : 'Hold to sign',
            style: TextStyle(
              fontSize: 11,
              fontWeight: _pushing ? FontWeight.w700 : FontWeight.normal,
              color:
                  _pushing ? cs.primary : cs.onSurface.withValues(alpha: 0.45),
            ),
          ),

          // ── Hold-to-Speak row (test mode only) ────────────────────
          if (call.isTestMode) ...[
            const SizedBox(height: 16),
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                GestureDetector(
                  onTapDown: (_) => _startSpeaking(),
                  onTapUp: (_) => _stopSpeaking(),
                  onTapCancel: _stopSpeaking,
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 120),
                    height: 56,
                    width: 56,
                    decoration: BoxDecoration(
                      color:
                          _speaking ? cs.tertiary : cs.surfaceContainerHighest,
                      shape: BoxShape.circle,
                      border: Border.all(
                        color: _speaking
                            ? cs.tertiary
                            : cs.outline.withValues(alpha: 0.5),
                        width: _speaking ? 2.5 : 1.5,
                      ),
                      boxShadow: _speaking
                          ? [
                              BoxShadow(
                                color: cs.tertiary.withValues(alpha: 0.35),
                                blurRadius: 16,
                                offset: const Offset(0, 4),
                              ),
                            ]
                          : null,
                    ),
                    child: Icon(
                      _speaking ? Icons.mic_rounded : Icons.mic_outlined,
                      color: _speaking
                          ? Colors.white
                          : cs.onSurface.withValues(alpha: 0.6),
                      size: 22,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              _speaking ? 'Listening...' : 'Hold to speak (STT test)',
              style: TextStyle(
                fontSize: 10,
                fontWeight: _speaking ? FontWeight.w700 : FontWeight.normal,
                color: _speaking
                    ? cs.tertiary
                    : cs.onSurface.withValues(alpha: 0.4),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _CaptionBubble extends StatelessWidget {
  const _CaptionBubble({required this.caption});
  final Caption caption;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    final isYou = caption.isFromSigner;

    final ts = caption.timestamp;
    final timeStr =
        '${ts.minute.toString().padLeft(2, '0')}:${ts.second.toString().padLeft(2, '0')}';

    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Column(
        crossAxisAlignment:
            isYou ? CrossAxisAlignment.end : CrossAxisAlignment.start,
        children: [
          Text(
            '${isYou ? 'You (Sign)' : 'Caller'} · $timeStr',
            style: TextStyle(
              fontSize: 10,
              color: cs.onSurface.withValues(alpha: 0.45),
            ),
          ),
          const SizedBox(height: 3),
          Container(
            constraints: const BoxConstraints(maxWidth: 280),
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
            decoration: BoxDecoration(
              color: isYou ? cs.primary : cs.surfaceContainerHighest,
              borderRadius: BorderRadius.only(
                topLeft: const Radius.circular(16),
                topRight: const Radius.circular(16),
                bottomLeft: Radius.circular(isYou ? 16 : 4),
                bottomRight: Radius.circular(isYou ? 4 : 16),
              ),
            ),
            child: Text(
              caption.text,
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w500,
                color: isYou ? Colors.white : cs.onSurface,
              ),
            ),
          ),
        ],
      ),
    ).animate().fade(duration: 250.ms).slideY(begin: 0.1, duration: 250.ms);
  }
}

class _ConnectedBadge extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: cs.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: cs.outline.withValues(alpha: 0.4)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            height: 6,
            width: 6,
            decoration: const BoxDecoration(
              color: AppColors.success,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 5),
          const Text(
            'Connected',
            style: TextStyle(fontSize: 10, fontWeight: FontWeight.w600),
          ),
        ],
      ),
    );
  }
}

class _RecBadgeSmall extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: AppColors.destructive.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(12),
      ),
      child: const Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.circle, size: 6, color: AppColors.destructive),
          SizedBox(width: 4),
          Text(
            'REC',
            style: TextStyle(
              fontSize: 9,
              fontWeight: FontWeight.w800,
              color: AppColors.destructive,
            ),
          ),
        ],
      ),
    );
  }
}
