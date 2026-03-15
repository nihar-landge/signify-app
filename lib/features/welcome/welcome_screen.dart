import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/providers/providers.dart';
import '../../core/theme/app_theme.dart';

class WelcomeScreen extends ConsumerStatefulWidget {
  const WelcomeScreen({super.key});

  @override
  ConsumerState<WelcomeScreen> createState() => _WelcomeScreenState();
}

class _WelcomeScreenState extends ConsumerState<WelcomeScreen> {
  String? _selectedLanguage;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;

    return Scaffold(
      body: SafeArea(
        child: Stack(
          children: [
            // Theme toggle top-right
            Positioned(
              top: 12,
              right: 16,
              child: _ThemeSwitchButton(),
            ),

            // Main content
            Center(
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(horizontal: 28),
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 400),
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      const SizedBox(height: 60),

                      // ── Logo ──────────────────────────────────────────────
                      Stack(
                        clipBehavior: Clip.none,
                        children: [
                          Container(
                            height: 96,
                            width: 96,
                            decoration: BoxDecoration(
                              color: cs.primary,
                              borderRadius: BorderRadius.circular(28),
                              boxShadow: [
                                BoxShadow(
                                  color: cs.primary.withValues(alpha: 0.3),
                                  blurRadius: 20,
                                  offset: const Offset(0, 8),
                                ),
                              ],
                            ),
                            child: const Icon(
                              Icons.back_hand_rounded,
                              color: Colors.white,
                              size: 48,
                            ),
                          ),
                          Positioned(
                            bottom: -4,
                            right: -4,
                            child: Container(
                              height: 24,
                              width: 24,
                              decoration: BoxDecoration(
                                color: AppColors.success,
                                borderRadius: BorderRadius.circular(8),
                              ),
                            ),
                          ),
                        ],
                      )
                          .animate()
                          .scale(
                            begin: const Offset(0.8, 0.8),
                            duration: 600.ms,
                            curve: Curves.easeOutBack,
                          )
                          .fade(duration: 400.ms),

                      const SizedBox(height: 32),

                      // ── Title ─────────────────────────────────────────────
                      Column(
                        children: [
                          Text(
                            'Signify',
                            style: Theme.of(context)
                                .textTheme
                                .displayLarge
                                ?.copyWith(fontSize: 40),
                          ),
                          const SizedBox(height: 8),
                          Text(
                            'Bridge the gap between sign and speech',
                            style: Theme.of(context)
                                .textTheme
                                .bodyMedium
                                ?.copyWith(
                                  color: cs.onSurface.withValues(alpha: 0.55),
                                ),
                            textAlign: TextAlign.center,
                          ),
                        ],
                      )
                          .animate(delay: 200.ms)
                          .fade(duration: 500.ms)
                          .slideY(begin: 0.2, duration: 500.ms),

                      const SizedBox(height: 40),

                      // ── Language Selector ─────────────────────────────────
                      Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              Icon(
                                Icons.language_rounded,
                                size: 16,
                                color: cs.onSurface.withValues(alpha: 0.5),
                              ),
                              const SizedBox(width: 6),
                              Text(
                                'Choose your sign language',
                                style: Theme.of(context)
                                    .textTheme
                                    .bodySmall
                                    ?.copyWith(
                                      color:
                                          cs.onSurface.withValues(alpha: 0.5),
                                    ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 12),
                          Row(
                            children: [
                              _LanguageCard(
                                code: 'ASL',
                                fullName: 'American Sign Language',
                                selected: _selectedLanguage == 'ASL',
                                onTap: () =>
                                    setState(() => _selectedLanguage = 'ASL'),
                              ),
                              const SizedBox(width: 12),
                              _LanguageCard(
                                code: 'ISL',
                                fullName: 'Indian Sign Language',
                                selected: _selectedLanguage == 'ISL',
                                onTap: () =>
                                    setState(() => _selectedLanguage = 'ISL'),
                              ),
                            ],
                          ),
                        ],
                      )
                          .animate(delay: 400.ms)
                          .fade(duration: 500.ms)
                          .slideY(begin: 0.2, duration: 500.ms),

                      const SizedBox(height: 32),

                      // ── Get Started Button ────────────────────────────────
                      SizedBox(
                        width: double.infinity,
                        child: ElevatedButton.icon(
                          onPressed: _selectedLanguage == null
                              ? null
                              : () async {
                                  await ref
                                      .read(settingsProvider.notifier)
                                      .setLanguage(_selectedLanguage!);
                                  if (context.mounted) context.go('/home');
                                },
                          icon: const Icon(Icons.arrow_forward_rounded),
                          label: const Text('Get Started'),
                          style: ElevatedButton.styleFrom(
                            padding: const EdgeInsets.symmetric(vertical: 18),
                            textStyle: const TextStyle(
                              fontSize: 16,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                        ),
                      )
                          .animate(delay: 600.ms)
                          .fade(duration: 500.ms)
                          .slideY(begin: 0.2, duration: 500.ms),

                      const SizedBox(height: 24),

                      Text(
                        'Real-time sign language recognition & voice calls',
                        style: Theme.of(context).textTheme.bodySmall?.copyWith(
                              color: cs.onSurface.withValues(alpha: 0.4),
                            ),
                        textAlign: TextAlign.center,
                      ).animate(delay: 700.ms).fade(duration: 400.ms),

                      const SizedBox(height: 40),
                    ],
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _LanguageCard extends StatelessWidget {
  const _LanguageCard({
    required this.code,
    required this.fullName,
    required this.selected,
    required this.onTap,
  });

  final String code;
  final String fullName;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;

    return Expanded(
      child: GestureDetector(
        onTap: onTap,
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 200),
          padding: const EdgeInsets.symmetric(vertical: 20, horizontal: 16),
          decoration: BoxDecoration(
            color: selected ? cs.primary.withValues(alpha: 0.08) : cs.surface,
            borderRadius: BorderRadius.circular(18),
            border: Border.all(
              color: selected ? cs.primary : cs.outline,
              width: selected ? 2 : 1,
            ),
            boxShadow: selected
                ? [
                    BoxShadow(
                      color: cs.primary.withValues(alpha: 0.12),
                      blurRadius: 12,
                      offset: const Offset(0, 4),
                    )
                  ]
                : [],
          ),
          child: Column(
            children: [
              Text(
                code,
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: selected ? cs.primary : cs.onSurface,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                fullName,
                style: TextStyle(
                  fontSize: 10,
                  color: cs.onSurface.withValues(alpha: 0.5),
                ),
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _ThemeSwitchButton extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    return IconButton(
      onPressed: () {
        ref.read(settingsProvider.notifier).setThemeMode(
              isDark ? ThemeMode.light : ThemeMode.dark,
            );
      },
      icon: Icon(
        isDark ? Icons.light_mode_rounded : Icons.dark_mode_rounded,
      ),
    );
  }
}
