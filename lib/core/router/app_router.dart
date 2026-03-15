import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../providers/providers.dart';

import '../../features/welcome/welcome_screen.dart';
import '../../features/home/home_screen.dart';
import '../../features/camera/camera_screen.dart';
import '../../features/call/call_screen.dart';
import '../../features/history/history_screen.dart';
import '../../features/settings/settings_screen.dart';

// ─── Shell with bottom nav ────────────────────────────────────────────────────

final _shellNavigatorKey = GlobalKey<NavigatorState>(debugLabel: 'shell');

final appRouter = GoRouter(
  initialLocation: '/',
  routes: [
    GoRoute(
      path: '/',
      builder: (context, state) => const WelcomeScreen(),
    ),
    ShellRoute(
      navigatorKey: _shellNavigatorKey,
      builder: (context, state, child) => AppShell(child: child),
      routes: [
        GoRoute(
          path: '/home',
          builder: (context, state) => const HomeScreen(),
        ),
        GoRoute(
          path: '/camera',
          builder: (context, state) => const CameraScreen(),
        ),
        GoRoute(
          path: '/call',
          builder: (context, state) => const CallScreen(),
        ),
        GoRoute(
          path: '/history',
          builder: (context, state) => const HistoryScreen(),
        ),
        GoRoute(
          path: '/settings',
          builder: (context, state) => const SettingsScreen(),
        ),
      ],
    ),
  ],
);

// ─── App shell (top-bar + bottom nav) ─────────────────────────────────────────

class AppShell extends ConsumerWidget {
  const AppShell({super.key, required this.child});

  final Widget child;

  static const _tabs = [
    _TabInfo(icon: Icons.home_rounded, label: 'Home', path: '/home'),
    _TabInfo(icon: Icons.camera_alt_rounded, label: 'Camera', path: '/camera'),
    _TabInfo(icon: Icons.phone_rounded, label: 'Call', path: '/call'),
    _TabInfo(icon: Icons.history_rounded, label: 'History', path: '/history'),
    _TabInfo(
        icon: Icons.settings_rounded, label: 'Settings', path: '/settings'),
  ];

  // P11 fix: cache computed index so repeated builds with the same location
  // don't re-run indexWhere on every navigation event.
  static final _indexCache = <String, int>{};

  static int _indexForPath(String location) {
    return _indexCache.putIfAbsent(location, () {
      final match = _tabs.indexWhere((t) => location.startsWith(t.path));
      return match < 0 ? 0 : match;
    });
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final location = GoRouterState.of(context).uri.toString();
    final currentIndex = _indexForPath(location);
    final cs = Theme.of(context).colorScheme;

    // Listen for incoming calls and show a dialog
    ref.listen(incomingCallStreamProvider, (_, next) {
      next.whenData((event) {
        // Backend AppEvent uses 'message' for the caller number and 'text'
        // for the session ID (matching the AppEvent struct JSON tags).
        final fromNumber = event['message'] as String? ??
            event['from'] as String? ??
            'Unknown';
        final sessionId =
            event['text'] as String? ?? event['sessionId'] as String? ?? '';
        showDialog<void>(
          context: context,
          barrierDismissible: false,
          builder: (_) => _IncomingCallDialog(
            fromNumber: fromNumber,
            sessionId: sessionId,
            onAccept: () {
              Navigator.of(context).pop();
              ref
                  .read(callProvider.notifier)
                  .acceptIncomingCall(sessionId, fromNumber);
              context.go('/call');
            },
            onDecline: () => Navigator.of(context).pop(),
          ),
        );
      });
    });

    return Scaffold(
      // ── Top Bar ──────────────────────────────────────────────────────────
      appBar: AppBar(
        automaticallyImplyLeading: false,
        title: Row(
          children: [
            Container(
              height: 36,
              width: 36,
              decoration: BoxDecoration(
                color: cs.primary,
                borderRadius: BorderRadius.circular(10),
              ),
              child: const Icon(Icons.back_hand_rounded,
                  color: Colors.white, size: 18),
            ),
            const SizedBox(width: 10),
            const Text('Signify'),
          ],
        ),
        actions: [
          _ThemeToggle(),
          const SizedBox(width: 8),
        ],
      ),

      body: child,

      // ── Bottom Navigation Bar ─────────────────────────────────────────────
      bottomNavigationBar: NavigationBar(
        selectedIndex: currentIndex,
        onDestinationSelected: (i) => context.go(_tabs[i].path),
        destinations: _tabs
            .map(
              (t) => NavigationDestination(
                icon: Icon(t.icon),
                label: t.label,
              ),
            )
            .toList(),
      ),
    );
  }
}

class _ThemeToggle extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    return IconButton(
      onPressed: () {
        ref.read(settingsProvider.notifier).setThemeMode(
              isDark ? ThemeMode.light : ThemeMode.dark,
            );
      },
      icon: Icon(isDark ? Icons.light_mode_rounded : Icons.dark_mode_rounded),
      tooltip: isDark ? 'Light mode' : 'Dark mode',
    );
  }
}

class _TabInfo {
  final IconData icon;
  final String label;
  final String path;
  const _TabInfo({required this.icon, required this.label, required this.path});
}

// ─── Incoming Call Dialog ─────────────────────────────────────────────────────

class _IncomingCallDialog extends StatelessWidget {
  const _IncomingCallDialog({
    required this.fromNumber,
    required this.sessionId,
    required this.onAccept,
    required this.onDecline,
  });

  final String fromNumber;
  final String sessionId;
  final VoidCallback onAccept;
  final VoidCallback onDecline;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return AlertDialog(
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            height: 56,
            width: 56,
            decoration: BoxDecoration(
              color: Colors.green.withValues(alpha: 0.12),
              shape: BoxShape.circle,
            ),
            child:
                const Icon(Icons.phone_rounded, color: Colors.green, size: 28),
          ),
          const SizedBox(height: 16),
          const Text(
            'Incoming Call',
            style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 6),
          Text(
            fromNumber,
            style: TextStyle(
                fontSize: 14, color: cs.onSurface.withValues(alpha: 0.7)),
          ),
        ],
      ),
      actionsAlignment: MainAxisAlignment.spaceEvenly,
      actions: [
        TextButton.icon(
          onPressed: onDecline,
          icon: const Icon(Icons.call_end_rounded, color: Colors.red),
          label: const Text('Decline', style: TextStyle(color: Colors.red)),
        ),
        TextButton.icon(
          onPressed: onAccept,
          icon: const Icon(Icons.phone_rounded, color: Colors.green),
          label: const Text('Accept', style: TextStyle(color: Colors.green)),
        ),
      ],
    );
  }
}
