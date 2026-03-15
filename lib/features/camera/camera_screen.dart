import 'dart:async';

import 'package:audioplayers/audioplayers.dart';
import 'package:camera/camera.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:permission_handler/permission_handler.dart';
import 'package:uuid/uuid.dart';

import '../../core/providers/providers.dart';
import '../../core/services/landmark_extractor.dart';
import '../../core/services/landmark_websocket_client.dart';
import '../../core/services/websocket_service.dart';
import '../../core/theme/app_theme.dart';

// ---------------------------------------------------------------------------
// Provider – use the shared availableCamerasProvider from providers.dart so
// that the platform channel is called only once across all screens (P8 fix).
// ---------------------------------------------------------------------------
final _availableCamerasProvider = availableCamerasProvider;

// ---------------------------------------------------------------------------
// CameraScreen
// ---------------------------------------------------------------------------
class CameraScreen extends ConsumerStatefulWidget {
  const CameraScreen({super.key});

  @override
  ConsumerState<CameraScreen> createState() => _CameraScreenState();
}

class _CameraScreenState extends ConsumerState<CameraScreen>
    with WidgetsBindingObserver {
  // Camera state
  CameraController? _controller;
  bool _cameraPermissionGranted = false;
  bool _cameraInitializing = false;
  bool _cameraError = false;
  String _cameraErrorMessage = '';
  int _selectedCameraIndex = 0;

  // Recognition state
  bool _recording = false;
  String _captionText = '';
  String _partialCaption = '';

  // Services
  LandmarkExtractor? _extractor;
  LandmarkWebSocketClient? _wsClient;
  AudioPlayer? _audioPlayer;

  // WebSocket stats for debug
  bool _wsConnected = false;

  static const _uuid = Uuid();

  // ---------------------------------------------------------------------------
  // Lifecycle
  // ---------------------------------------------------------------------------
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _requestCameraPermission();
  }

  @override
  void deactivate() {
    // Called when the widget is removed from the tree (tab navigation away).
    // Stop the image stream and release the pipeline so that the Call tab can
    // acquire the same physical camera without a CameraException.  We do NOT
    // dispose the CameraController here — that stays alive so the preview
    // resumes instantly when the user returns to this tab.
    if (_recording) {
      _stopRecordingCleanup();
      // Keep _recording = true so the UI reflects "was recording" on resume
      // and activate() can restart the stream automatically.
      _wsConnected = false;
    } else {
      // Not recording — still stop any lingering image stream to be safe.
      _controller?.stopImageStream().catchError((_) {});
    }
    super.deactivate();
  }

  @override
  void activate() {
    super.activate();
    // Called when this widget is re-inserted into the tree (tab navigated back).
    // If the user was recording before they left, restart the pipeline.
    if (_recording) {
      _recording = false; // reset so _startRecording() doesn't early-exit
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) _startRecording();
      });
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _stopRecordingCleanup();
    _controller?.dispose();
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

  // ---------------------------------------------------------------------------
  // Permission & camera init
  // ---------------------------------------------------------------------------
  Future<void> _requestCameraPermission() async {
    final status = await Permission.camera.request();
    if (!mounted) return;
    setState(() => _cameraPermissionGranted = status.isGranted);
    if (status.isGranted) {
      _pickAndInitCamera();
    }
  }

  void _pickAndInitCamera() {
    final cameras = ref.read(_availableCamerasProvider).valueOrNull;
    if (cameras == null || cameras.isEmpty) {
      setState(() {
        _cameraError = true;
        _cameraErrorMessage = 'No cameras found on this device.';
      });
      return;
    }

    final frontIndex = cameras.indexWhere(
      (c) => c.lensDirection == CameraLensDirection.front,
    );
    _selectedCameraIndex = frontIndex >= 0 ? frontIndex : 0;
    _initCamera(cameras[_selectedCameraIndex]);
  }

  Future<void> _initCamera(CameraDescription description) async {
    setState(() {
      _cameraInitializing = true;
      _cameraError = false;
    });

    final prev = _controller;
    if (prev != null) {
      await prev.dispose();
    }

    final controller = CameraController(
      description,
      ResolutionPreset.medium, // medium is sufficient for landmark detection
      enableAudio: false,
      imageFormatGroup: ImageFormatGroup.yuv420,
    );

    try {
      await controller.initialize();
      if (!mounted) {
        await controller.dispose();
        return;
      }
      setState(() {
        _controller = controller;
        _cameraInitializing = false;
      });
      // Pre-warm the landmark extractor so that ML Kit and the hand landmarker
      // GPU model load NOW (during camera init) instead of on the first Record
      // press, which would freeze the UI thread for 1-3 seconds.
      _extractor ??= LandmarkExtractor();
      // Fire-and-forget: initialize() is idempotent and lightweight here;
      // the heavy GPU work is deferred into the first detect() call which
      // happens in the background image-stream processing.
      unawaited(_extractor!.initialize());
      _audioPlayer ??= AudioPlayer();
    } on CameraException catch (e) {
      await controller.dispose();
      if (!mounted) return;
      setState(() {
        _cameraInitializing = false;
        _cameraError = true;
        _cameraErrorMessage = e.description ?? 'Camera initialisation failed.';
      });
    }
  }

  Future<void> _flipCamera() async {
    final cameras = ref.read(_availableCamerasProvider).valueOrNull;
    if (cameras == null || cameras.length < 2) return;
    _selectedCameraIndex = (_selectedCameraIndex + 1) % cameras.length;
    await _initCamera(cameras[_selectedCameraIndex]);
  }

  // ---------------------------------------------------------------------------
  // Recording / recognition controls
  // ---------------------------------------------------------------------------
  Future<void> _startRecording() async {
    if (_recording) return;
    final controller = _controller;
    if (controller == null || !controller.value.isInitialized) return;

    final language = ref.read(selectedLanguageProvider);
    final sessionId = 'test-session-${_uuid.v4().substring(0, 8)}';

    // Reuse the extractor pre-warmed by _initCamera; create one only if
    // somehow it wasn't initialised yet (e.g. camera permission was granted
    // very late).
    _extractor ??= LandmarkExtractor();
    await _extractor!.initialize();
    if (!mounted) return;

    _audioPlayer ??= AudioPlayer();

    // Init WebSocket client
    _wsClient = LandmarkWebSocketClient(
      serverUrl: SignifyConfig.rootUrl,
      sessionId: sessionId,
      requestLanguage: language,
    );

    _wsClient!.onConnected = () {
      if (mounted) setState(() => _wsConnected = true);
    };

    _wsClient!.onDisconnected = () {
      if (mounted) setState(() => _wsConnected = false);
    };

    _wsClient!.onSignEvent = (text, isFinal) {
      if (!mounted) return;
      setState(() {
        if (isFinal) {
          _captionText = text;
          _partialCaption = '';
        } else {
          _partialCaption = text;
        }
      });
    };

    _wsClient!.onAudioReceived = (Uint8List audioBytes) async {
      try {
        if (ref.read(settingsProvider).voiceOutput) {
          await _audioPlayer?.play(BytesSource(audioBytes));
        }
      } catch (e) {
        debugPrint('CameraScreen: audio playback error: $e');
      }
    };

    await _wsClient!.connect();
    if (!mounted) return;

    // Start image stream
    await controller.startImageStream((CameraImage image) {
      _onCameraFrame(image, controller.description);
    });
    if (!mounted) return;

    setState(() {
      _recording = true;
      _captionText = '';
      _partialCaption = '';
    });
  }

  void _onCameraFrame(CameraImage image, CameraDescription description) {
    // Fire-and-forget: extractor and client handle concurrency guards internally
    _extractor?.processImage(image, description).then((frame) {
      if (frame == null) return;
      final landmarks = frame.toFullHolisticLandmarks();
      final hasHands = frame.hasData;
      _wsClient?.sendLandmarks(
        present: hasHands,
        landmarks: hasHands ? landmarks : null,
      );
    });
  }

  void _stopRecordingCleanup() {
    _controller?.stopImageStream().catchError((_) {});
    _wsClient?.disconnect();
    _wsClient = null;
    _extractor?.dispose();
    _extractor = null;
    _audioPlayer?.dispose();
    _audioPlayer = null;
  }

  void _stopRecording() {
    _stopRecordingCleanup();
    setState(() {
      _recording = false;
      _wsConnected = false;
    });
  }

  void _clearCaption() {
    setState(() {
      _captionText = '';
      _partialCaption = '';
    });
  }

  void _copyCaption() {
    final text = _captionText.isNotEmpty ? _captionText : _partialCaption;
    if (text.isEmpty) return;
    Clipboard.setData(ClipboardData(text: text));
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text('Copied to clipboard'),
        behavior: SnackBarBehavior.floating,
        duration: Duration(seconds: 2),
      ),
    );
  }

  // ---------------------------------------------------------------------------
  // Build
  // ---------------------------------------------------------------------------
  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    final language = ref.watch(selectedLanguageProvider);

    // Trigger camera init once cameras are loaded (if permission already granted).
    // Deferred to post-frame to avoid calling setState() during a build pass.
    ref.listen(_availableCamerasProvider, (_, next) {
      if (next.hasValue && _cameraPermissionGranted && _controller == null) {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (mounted) _pickAndInitCamera();
        });
      }
    });

    // Caption to display (prefer final, fall back to partial)
    final displayCaption =
        _captionText.isNotEmpty ? _captionText : _partialCaption;
    final isPartial = _captionText.isEmpty && _partialCaption.isNotEmpty;

    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 520),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // ── Status row ────────────────────────────────────────────────
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                _StatusChip(label: '$language Mode'),
                Row(
                  children: [
                    if (_recording && _wsConnected)
                      _StatusChip(
                        label: '● Live',
                        active: true,
                      )
                    else if (_recording)
                      _StatusChip(
                        label: '⟳ Connecting',
                        active: false,
                      )
                    else
                      _StatusChip(label: 'Ready'),
                  ],
                ),
              ],
            ).animate().fade(duration: 300.ms),

            const SizedBox(height: 14),

            // ── Camera viewport ───────────────────────────────────────────
            AspectRatio(
              aspectRatio: 3 / 4,
              child: Container(
                clipBehavior: Clip.hardEdge,
                decoration: BoxDecoration(
                  color: cs.surface,
                  borderRadius: BorderRadius.circular(24),
                  border: Border.all(color: cs.outline.withValues(alpha: 0.5)),
                  boxShadow: [
                    BoxShadow(
                      color: Colors.black.withValues(alpha: 0.06),
                      blurRadius: 12,
                      offset: const Offset(0, 4),
                    ),
                  ],
                ),
                child: Stack(
                  fit: StackFit.expand,
                  children: [
                    _buildCameraPreview(cs),
                    if (_recording)
                      const Positioned(
                        left: 16,
                        top: 16,
                        child: _RecBadge(),
                      ),
                    if (_cameraPermissionGranted &&
                        !_cameraError &&
                        !_cameraInitializing)
                      Positioned(
                        right: 12,
                        bottom: 12,
                        child: _IconOverlayBtn(
                          icon: Icons.flip_camera_ios_rounded,
                          onTap: _flipCamera,
                          tooltip: 'Flip camera',
                        ),
                      ),
                  ],
                ),
              ),
            ).animate(delay: 80.ms).fade(duration: 400.ms),

            const SizedBox(height: 14),

            // ── Caption bar ───────────────────────────────────────────────
            AnimatedContainer(
              duration: const Duration(milliseconds: 300),
              width: double.infinity,
              constraints: const BoxConstraints(minHeight: 72),
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: cs.surface,
                borderRadius: BorderRadius.circular(18),
                border: Border.all(
                  color: isPartial
                      ? cs.primary.withValues(alpha: 0.4)
                      : cs.outline.withValues(alpha: 0.5),
                ),
                boxShadow: [
                  BoxShadow(
                    color: Colors.black.withValues(alpha: 0.04),
                    blurRadius: 8,
                  ),
                ],
              ),
              child: AnimatedSwitcher(
                duration: const Duration(milliseconds: 300),
                child: displayCaption.isNotEmpty
                    ? Text(
                        displayCaption,
                        key: ValueKey(displayCaption),
                        style: TextStyle(
                          fontSize: 18,
                          fontWeight: FontWeight.w600,
                          color: isPartial
                              ? cs.onSurface.withValues(alpha: 0.6)
                              : cs.onSurface,
                          fontStyle:
                              isPartial ? FontStyle.italic : FontStyle.normal,
                        ),
                        textAlign: TextAlign.center,
                      )
                    : Text(
                        key: const ValueKey('placeholder'),
                        _recording
                            ? 'Waiting for signs...'
                            : 'Start recognition to see captions',
                        style: TextStyle(
                          fontSize: 13,
                          color: cs.onSurface.withValues(alpha: 0.4),
                        ),
                        textAlign: TextAlign.center,
                      ),
              ),
            ).animate(delay: 120.ms).fade(duration: 400.ms),

            const SizedBox(height: 20),

            // ── Controls ──────────────────────────────────────────────────
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                _ControlBtn(
                  icon: Icons.delete_outline_rounded,
                  onTap: _clearCaption,
                  tooltip: 'Clear',
                ),
                const SizedBox(width: 20),
                GestureDetector(
                  onTap: _recording ? _stopRecording : _startRecording,
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 200),
                    height: 64,
                    width: 64,
                    decoration: BoxDecoration(
                      color: _recording ? AppColors.destructive : cs.primary,
                      shape: BoxShape.circle,
                      boxShadow: [
                        BoxShadow(
                          color:
                              (_recording ? AppColors.destructive : cs.primary)
                                  .withValues(alpha: 0.3),
                          blurRadius: 16,
                          offset: const Offset(0, 6),
                        ),
                      ],
                    ),
                    child: Icon(
                      _recording
                          ? Icons.videocam_off_rounded
                          : Icons.videocam_rounded,
                      color: Colors.white,
                      size: 28,
                    ),
                  ),
                ),
                const SizedBox(width: 20),
                _ControlBtn(
                  icon: Icons.copy_rounded,
                  onTap: _copyCaption,
                  tooltip: 'Copy',
                ),
              ],
            ).animate(delay: 160.ms).fade(duration: 400.ms),

            const SizedBox(height: 16),

            // ── Switch to call ────────────────────────────────────────────
            Center(
              child: OutlinedButton.icon(
                onPressed: () => context.go('/call'),
                icon: const Icon(Icons.phone_rounded, size: 16),
                label: const Text('Switch to Call'),
                style: OutlinedButton.styleFrom(
                  shape: const StadiumBorder(),
                  padding:
                      const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
                ),
              ),
            ).animate(delay: 200.ms).fade(duration: 400.ms),
          ],
        ),
      ),
    );
  }

  // ---------------------------------------------------------------------------
  // Camera preview
  // ---------------------------------------------------------------------------
  Widget _buildCameraPreview(ColorScheme cs) {
    if (!_cameraPermissionGranted) {
      return _CameraPlaceholder(
        icon: Icons.no_photography_rounded,
        message: 'Camera permission required',
        actionLabel: 'Grant Permission',
        onAction: _requestCameraPermission,
        cs: cs,
      );
    }

    if (_cameraInitializing) {
      return Center(
        child: CircularProgressIndicator(color: cs.primary),
      );
    }

    if (_cameraError) {
      return _CameraPlaceholder(
        icon: Icons.error_outline_rounded,
        message: _cameraErrorMessage,
        actionLabel: 'Retry',
        onAction: _pickAndInitCamera,
        cs: cs,
      );
    }

    final controller = _controller;
    if (controller == null || !controller.value.isInitialized) {
      return _CameraPlaceholder(
        icon: Icons.camera_alt_rounded,
        message: 'Initialising camera…',
        cs: cs,
      );
    }

    return ClipRRect(
      borderRadius: BorderRadius.circular(24),
      child: FittedBox(
        fit: BoxFit.cover,
        child: SizedBox(
          width: controller.value.previewSize?.height ?? 1,
          height: controller.value.previewSize?.width ?? 1,
          child: CameraPreview(controller),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

class _CameraPlaceholder extends StatelessWidget {
  const _CameraPlaceholder({
    required this.icon,
    required this.message,
    required this.cs,
    this.actionLabel,
    this.onAction,
  });

  final IconData icon;
  final String message;
  final ColorScheme cs;
  final String? actionLabel;
  final VoidCallback? onAction;

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Container(
          height: 80,
          width: 80,
          decoration: BoxDecoration(
            color: cs.surfaceContainerHighest,
            borderRadius: BorderRadius.circular(20),
          ),
          child:
              Icon(icon, size: 40, color: cs.onSurface.withValues(alpha: 0.3)),
        ),
        const SizedBox(height: 12),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Text(
            message,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 13,
              color: cs.onSurface.withValues(alpha: 0.5),
            ),
          ),
        ),
        if (actionLabel != null && onAction != null) ...[
          const SizedBox(height: 12),
          FilledButton.tonal(
            onPressed: onAction,
            child: Text(actionLabel!),
          ),
        ],
      ],
    );
  }
}

class _IconOverlayBtn extends StatelessWidget {
  const _IconOverlayBtn({
    required this.icon,
    required this.onTap,
    required this.tooltip,
  });
  final IconData icon;
  final VoidCallback onTap;
  final String tooltip;

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: tooltip,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          height: 40,
          width: 40,
          decoration: BoxDecoration(
            color: Colors.black.withValues(alpha: 0.45),
            shape: BoxShape.circle,
          ),
          child: Icon(icon, size: 20, color: Colors.white),
        ),
      ),
    );
  }
}

class _StatusChip extends StatelessWidget {
  const _StatusChip({required this.label, this.active = false});
  final String label;
  final bool active;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: active
            ? cs.primary.withValues(alpha: 0.12)
            : cs.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(
          color: active
              ? cs.primary.withValues(alpha: 0.4)
              : cs.outline.withValues(alpha: 0.5),
        ),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w600,
          color: active ? cs.primary : cs.onSurface.withValues(alpha: 0.7),
        ),
      ),
    );
  }
}

class _RecBadge extends StatefulWidget {
  const _RecBadge();

  @override
  State<_RecBadge> createState() => _RecBadgeState();
}

class _RecBadgeState extends State<_RecBadge>
    with SingleTickerProviderStateMixin {
  late AnimationController _ctrl;
  late Animation<double> _opacity;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      duration: const Duration(milliseconds: 800),
      vsync: this,
    )..repeat(reverse: true);
    _opacity = Tween<double>(begin: 1.0, end: 0.2).animate(_ctrl);
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: AppColors.destructive.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          FadeTransition(
            opacity: _opacity,
            child: Container(
              height: 8,
              width: 8,
              decoration: const BoxDecoration(
                color: AppColors.destructive,
                shape: BoxShape.circle,
              ),
            ),
          ),
          const SizedBox(width: 5),
          const Text(
            'REC',
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w800,
              color: AppColors.destructive,
            ),
          ),
        ],
      ),
    );
  }
}

class _ControlBtn extends StatelessWidget {
  const _ControlBtn({
    required this.icon,
    required this.onTap,
    required this.tooltip,
  });
  final IconData icon;
  final VoidCallback onTap;
  final String tooltip;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Tooltip(
      message: tooltip,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          height: 48,
          width: 48,
          decoration: BoxDecoration(
            color: cs.surface,
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: cs.outline.withValues(alpha: 0.5)),
          ),
          child: Icon(
            icon,
            size: 20,
            color: cs.onSurface.withValues(alpha: 0.7),
          ),
        ),
      ),
    );
  }
}
