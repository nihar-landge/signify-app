import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/providers/providers.dart';
import '../../core/theme/app_theme.dart';

class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final cs = Theme.of(context).colorScheme;
    final history = ref.watch(historyProvider);
    final recent = history.conversations.take(3).toList();

    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 32),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 520),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // ── Greeting ──────────────────────────────────────────────────
            Row(
              children: [
                Text(
                  'Welcome back',
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
                const SizedBox(width: 8),
                const Icon(Icons.auto_awesome_rounded,
                    size: 22, color: AppColors.primary),
              ],
            ).animate().fade(duration: 400.ms).slideY(begin: 0.15),
            const SizedBox(height: 4),
            Text(
              'What would you like to do today?',
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                    color: cs.onSurface.withValues(alpha: 0.55),
                  ),
            ).animate(delay: 80.ms).fade(duration: 400.ms),

            const SizedBox(height: 24),

            // ── Quick Actions ──────────────────────────────────────────────
            _QuickActions().animate(delay: 120.ms).fade(duration: 400.ms),

            const SizedBox(height: 28),

            // ── Recent Sessions ────────────────────────────────────────────
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(
                  'RECENT',
                  style: Theme.of(context).textTheme.labelSmall?.copyWith(
                        letterSpacing: 1.2,
                        fontWeight: FontWeight.w700,
                      ),
                ),
                GestureDetector(
                  onTap: () => context.go('/history'),
                  child: Row(
                    children: [
                      Text(
                        'View all',
                        style: TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w600,
                          color: cs.primary,
                        ),
                      ),
                      const SizedBox(width: 2),
                      Icon(Icons.arrow_forward_rounded,
                          size: 14, color: cs.primary),
                    ],
                  ),
                ),
              ],
            ).animate(delay: 160.ms).fade(duration: 400.ms),

            const SizedBox(height: 12),

            if (history.loading)
              const Center(
                child: Padding(
                  padding: EdgeInsets.all(24),
                  child: CircularProgressIndicator(),
                ),
              )
            else if (recent.isEmpty)
              Center(
                child: Padding(
                  padding: const EdgeInsets.all(24),
                  child: Text(
                    'No sessions yet. Start signing!',
                    style: TextStyle(
                      color: cs.onSurface.withValues(alpha: 0.45),
                      fontSize: 13,
                    ),
                  ),
                ),
              )
            else
              ...recent.asMap().entries.map(
                    (e) => Padding(
                      padding: const EdgeInsets.only(bottom: 10),
                      child: _SessionCard(session: e.value),
                    )
                        .animate(delay: (200 + e.key * 60).ms)
                        .fade(duration: 350.ms)
                        .slideY(begin: 0.15),
                  ),
          ],
        ),
      ),
    );
  }
}

// ─── Quick Action Cards ────────────────────────────────────────────────────────

class _QuickActions extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;

    final actions = [
      _ActionItem(
        icon: Icons.camera_alt_rounded,
        label: 'Start Signing',
        desc: 'Camera recognition',
        path: '/camera',
        iconColor: AppColors.primary,
        bgColor: AppColors.primary.withValues(alpha: 0.12),
        gradientStart: AppColors.primary.withValues(alpha: 0.12),
        gradientEnd: AppColors.primary.withValues(alpha: 0.04),
      ),
      _ActionItem(
        icon: Icons.phone_rounded,
        label: 'Make a Call',
        desc: 'Voice captions',
        path: '/call',
        iconColor: AppColors.success,
        bgColor: AppColors.success.withValues(alpha: 0.12),
        gradientStart: AppColors.success.withValues(alpha: 0.12),
        gradientEnd: AppColors.success.withValues(alpha: 0.04),
      ),
      _ActionItem(
        icon: Icons.history_rounded,
        label: 'History',
        desc: 'Past sessions',
        path: '/history',
        iconColor: cs.onSurface.withValues(alpha: 0.7),
        bgColor: cs.surfaceContainerHighest,
        gradientStart: cs.surfaceContainerHighest,
        gradientEnd: cs.surfaceContainerHighest.withValues(alpha: 0.5),
      ),
    ];

    return Row(
      children: actions
          .map((a) => Expanded(
                  child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 4),
                child: _ActionCard(item: a),
              )))
          .toList(),
    );
  }
}

class _ActionCard extends StatelessWidget {
  const _ActionCard({required this.item});
  final _ActionItem item;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;

    return GestureDetector(
      onTap: () => context.go(item.path),
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 16, horizontal: 8),
        decoration: BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [item.gradientStart, item.gradientEnd],
          ),
          borderRadius: BorderRadius.circular(18),
          border: Border.all(color: cs.outline.withValues(alpha: 0.5)),
        ),
        child: Column(
          children: [
            Container(
              height: 48,
              width: 48,
              decoration: BoxDecoration(
                color: item.bgColor,
                borderRadius: BorderRadius.circular(14),
              ),
              child: Icon(item.icon, color: item.iconColor, size: 22),
            ),
            const SizedBox(height: 10),
            Text(
              item.label,
              style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w700),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 2),
            Text(
              item.desc,
              style: TextStyle(
                fontSize: 9,
                color: cs.onSurface.withValues(alpha: 0.5),
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }
}

class _ActionItem {
  final IconData icon;
  final String label;
  final String desc;
  final String path;
  final Color iconColor;
  final Color bgColor;
  final Color gradientStart;
  final Color gradientEnd;

  const _ActionItem({
    required this.icon,
    required this.label,
    required this.desc,
    required this.path,
    required this.iconColor,
    required this.bgColor,
    required this.gradientStart,
    required this.gradientEnd,
  });
}

// ─── Session Card ──────────────────────────────────────────────────────────────

class _SessionCard extends StatelessWidget {
  const _SessionCard({required this.session});
  final ConversationSummary session;

  String _formatDuration(int seconds) {
    if (seconds < 60) return '${seconds}s';
    final m = seconds ~/ 60;
    final s = seconds % 60;
    return '${m}m ${s}s';
  }

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    final durationStr = _formatDuration(session.durationSeconds);

    return GestureDetector(
      onTap: () => context.go('/history'),
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: cs.surface,
          borderRadius: BorderRadius.circular(16),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.05),
              blurRadius: 8,
              offset: const Offset(0, 2),
            ),
          ],
        ),
        child: Row(
          children: [
            Container(
              height: 44,
              width: 44,
              decoration: BoxDecoration(
                color: cs.surfaceContainerHighest,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(Icons.phone_rounded,
                  size: 18, color: cs.onSurface.withValues(alpha: 0.6)),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Text(
                        '${session.date.year}-'
                        '${session.date.month.toString().padLeft(2, '0')}-'
                        '${session.date.day.toString().padLeft(2, '0')}',
                        style: const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      const SizedBox(width: 8),
                      _LangBadge(language: session.language),
                    ],
                  ),
                  const SizedBox(height: 3),
                  Text(
                    session.preview,
                    style: TextStyle(
                      fontSize: 11,
                      color: cs.onSurface.withValues(alpha: 0.5),
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            Text(
              durationStr,
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w500,
                color: cs.onSurface.withValues(alpha: 0.45),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _LangBadge extends StatelessWidget {
  const _LangBadge({required this.language});
  final String language;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: cs.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(
        language,
        style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w600),
      ),
    );
  }
}
