import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/providers/providers.dart';
import '../../core/theme/app_theme.dart';

class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final cs = Theme.of(context).colorScheme;
    final settings = ref.watch(settingsProvider);
    final notifier = ref.read(settingsProvider.notifier);
    final isDark = settings.themeMode == ThemeMode.dark;

    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 40),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 520),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Settings',
              style: Theme.of(context).textTheme.headlineMedium,
            ).animate().fade(duration: 350.ms),

            const SizedBox(height: 20),

            // ── Sign Language ────────────────────────────────────────────
            _SettingsCard(
              icon: Icons.translate_rounded,
              iconColor: AppColors.primary,
              iconBg: AppColors.primary.withValues(alpha: 0.1),
              title: 'Sign Language',
              child: Row(
                children: ['ASL', 'ISL'].map((lang) {
                  final selected = settings.language == lang;
                  return Expanded(
                    child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 4),
                      child: GestureDetector(
                        onTap: () => notifier.setLanguage(lang),
                        child: AnimatedContainer(
                          duration: const Duration(milliseconds: 180),
                          padding: const EdgeInsets.symmetric(vertical: 14),
                          decoration: BoxDecoration(
                            color: selected
                                ? cs.primary.withValues(alpha: 0.08)
                                : cs.surfaceContainerHighest,
                            borderRadius: BorderRadius.circular(12),
                            border: Border.all(
                              color: selected
                                  ? cs.primary
                                  : cs.outline.withValues(alpha: 0.4),
                              width: selected ? 2 : 1,
                            ),
                          ),
                          child: Text(
                            lang,
                            textAlign: TextAlign.center,
                            style: TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w700,
                              color: selected
                                  ? cs.primary
                                  : cs.onSurface.withValues(alpha: 0.6),
                            ),
                          ),
                        ),
                      ),
                    ),
                  );
                }).toList(),
              ),
            ).animate(delay: 60.ms).fade(duration: 350.ms),

            const SizedBox(height: 12),

            // ── Spoken Language (STT) ────────────────────────────────────
            _SettingsCard(
              icon: Icons.mic_rounded,
              iconColor: AppColors.primary,
              iconBg: AppColors.primary.withValues(alpha: 0.1),
              title: 'Caller\'s Spoken Language',
              subtitle: 'Used by Deepgram STT for voice captions',
              child: Row(
                children: [
                  ('en-US', 'English'),
                  ('hi', 'Hindi'),
                  ('mr', 'Marathi'),
                ].map((entry) {
                  final code = entry.$1;
                  final label = entry.$2;
                  final selected = settings.sttLanguage == code;
                  return Expanded(
                    child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 4),
                      child: GestureDetector(
                        onTap: () => notifier.setSttLanguage(code),
                        child: AnimatedContainer(
                          duration: const Duration(milliseconds: 180),
                          padding: const EdgeInsets.symmetric(vertical: 14),
                          decoration: BoxDecoration(
                            color: selected
                                ? cs.primary.withValues(alpha: 0.08)
                                : cs.surfaceContainerHighest,
                            borderRadius: BorderRadius.circular(12),
                            border: Border.all(
                              color: selected
                                  ? cs.primary
                                  : cs.outline.withValues(alpha: 0.4),
                              width: selected ? 2 : 1,
                            ),
                          ),
                          child: Text(
                            label,
                            textAlign: TextAlign.center,
                            style: TextStyle(
                              fontSize: 12,
                              fontWeight: FontWeight.w700,
                              color: selected
                                  ? cs.primary
                                  : cs.onSurface.withValues(alpha: 0.6),
                            ),
                          ),
                        ),
                      ),
                    ),
                  );
                }).toList(),
              ),
            ).animate(delay: 80.ms).fade(duration: 350.ms),

            const SizedBox(height: 12),

            // ── Voice Output ─────────────────────────────────────────────
            _SettingsCard(
              icon: Icons.volume_up_rounded,
              iconColor: AppColors.success,
              iconBg: AppColors.success.withValues(alpha: 0.1),
              title: 'Voice Output',
              subtitle: 'Read your signs aloud',
              trailing: Switch(
                value: settings.voiceOutput,
                onChanged: notifier.setVoiceOutput,
              ),
            ).animate(delay: 100.ms).fade(duration: 350.ms),

            const SizedBox(height: 12),

            // ── Dark Mode ────────────────────────────────────────────────
            _SettingsCard(
              icon: isDark ? Icons.dark_mode_rounded : Icons.light_mode_rounded,
              iconColor: cs.onSurface.withValues(alpha: 0.7),
              iconBg: cs.surfaceContainerHighest,
              title: 'Dark Mode',
              subtitle: 'Toggle light / dark theme',
              trailing: Switch(
                value: isDark,
                onChanged: (v) =>
                    notifier.setThemeMode(v ? ThemeMode.dark : ThemeMode.light),
              ),
            ).animate(delay: 140.ms).fade(duration: 350.ms),

            const SizedBox(height: 12),

            // ── Caption Size ─────────────────────────────────────────────
            _SettingsCard(
              icon: Icons.text_fields_rounded,
              iconColor: cs.onSurface.withValues(alpha: 0.7),
              iconBg: cs.surfaceContainerHighest,
              title: 'Caption Size',
              trailing: Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                decoration: BoxDecoration(
                  color: cs.surfaceContainerHighest,
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Text(
                  '${settings.captionFontSize.toInt()}px',
                  style: const TextStyle(
                      fontSize: 12, fontWeight: FontWeight.w600),
                ),
              ),
              child: Column(
                children: [
                  const SizedBox(height: 4),
                  Slider(
                    value: settings.captionFontSize,
                    min: 14,
                    max: 28,
                    divisions: 14,
                    onChanged: notifier.setCaptionFontSize,
                  ),
                  Container(
                    width: double.infinity,
                    padding: const EdgeInsets.symmetric(
                        vertical: 14, horizontal: 16),
                    decoration: BoxDecoration(
                      color: cs.surfaceContainerHighest,
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Text(
                      'Preview caption text',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: settings.captionFontSize,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ),
                ],
              ),
            ).animate(delay: 180.ms).fade(duration: 350.ms),

            const SizedBox(height: 12),

            // ── Backend Config ─────────────────────────────────────────
            _BackendConfigCard(settings: settings, notifier: notifier)
                .animate(delay: 200.ms)
                .fade(duration: 350.ms),

            const SizedBox(height: 12),

            // ── About ──────────────────────────────────────────────────
            _SettingsCard(
              icon: Icons.info_outline_rounded,
              iconColor: cs.onSurface.withValues(alpha: 0.5),
              iconBg: cs.surfaceContainerHighest,
              title: 'About',
              child: Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Divider(
                        height: 1, color: cs.outline.withValues(alpha: 0.4)),
                    const SizedBox(height: 10),
                    _AboutRow(label: 'Version', value: '1.0.0'),
                    _AboutRow(
                      label: 'Description',
                      value: 'Sign language recognition & voice communication',
                    ),
                    _AboutRow(
                      label: 'Powered by',
                      value: 'Go WebSocket · ML Kit · Twilio',
                    ),
                  ],
                ),
              ),
            ).animate(delay: 240.ms).fade(duration: 350.ms),
          ],
        ),
      ),
    );
  }
}

// ─── Settings Card ─────────────────────────────────────────────────────────────

class _SettingsCard extends StatelessWidget {
  const _SettingsCard({
    required this.icon,
    required this.iconColor,
    required this.iconBg,
    required this.title,
    this.subtitle,
    this.trailing,
    this.child,
  });

  final IconData icon;
  final Color iconColor;
  final Color iconBg;
  final String title;
  final String? subtitle;
  final Widget? trailing;
  final Widget? child;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: cs.surface,
        borderRadius: BorderRadius.circular(18),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.04),
            blurRadius: 8,
            offset: const Offset(0, 2),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                height: 36,
                width: 36,
                decoration: BoxDecoration(
                  color: iconBg,
                  borderRadius: BorderRadius.circular(10),
                ),
                child: Icon(icon, color: iconColor, size: 18),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: const TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    if (subtitle != null) ...[
                      const SizedBox(height: 2),
                      Text(
                        subtitle!,
                        style: TextStyle(
                          fontSize: 11,
                          color: cs.onSurface.withValues(alpha: 0.5),
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              if (trailing != null) trailing!,
            ],
          ),
          if (child != null) ...[
            const SizedBox(height: 12),
            child!,
          ],
        ],
      ),
    );
  }
}

class _AboutRow extends StatelessWidget {
  const _AboutRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 90,
            child: Text(
              label,
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w600,
                color: cs.onSurface.withValues(alpha: 0.5),
              ),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: const TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Backend Config Card ───────────────────────────────────────────────────────

class _BackendConfigCard extends StatefulWidget {
  const _BackendConfigCard({
    required this.settings,
    required this.notifier,
  });
  final AppSettings settings;
  final SettingsNotifier notifier;

  @override
  State<_BackendConfigCard> createState() => _BackendConfigCardState();
}

class _BackendConfigCardState extends State<_BackendConfigCard> {
  late TextEditingController _hostCtrl;
  late TextEditingController _localIPCtrl;
  late bool _useNgrok;

  @override
  void initState() {
    super.initState();
    _hostCtrl = TextEditingController(text: widget.settings.ngrokHost);
    _localIPCtrl = TextEditingController(text: widget.settings.localIP);
    _useNgrok = widget.settings.useNgrok;
  }

  // P19 fix: keep local controller in sync when parent widget rebuilds with
  // updated settings (e.g. settings loaded from prefs after widget mounts).
  @override
  void didUpdateWidget(_BackendConfigCard oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.settings.ngrokHost != widget.settings.ngrokHost) {
      // Only update when cursor is not active to avoid fighting with the user.
      if (!_hostCtrl.selection.isValid ||
          _hostCtrl.selection.baseOffset == _hostCtrl.selection.extentOffset) {
        _hostCtrl.text = widget.settings.ngrokHost;
      }
    }
    if (oldWidget.settings.localIP != widget.settings.localIP) {
      if (!_localIPCtrl.selection.isValid ||
          _localIPCtrl.selection.baseOffset ==
              _localIPCtrl.selection.extentOffset) {
        _localIPCtrl.text = widget.settings.localIP;
      }
    }
    if (oldWidget.settings.useNgrok != widget.settings.useNgrok) {
      _useNgrok = widget.settings.useNgrok;
    }
  }

  @override
  void dispose() {
    _hostCtrl.dispose();
    _localIPCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    // Compute the active URL for display so the user always knows what
    // IP:port will be used for the next connection.
    final activeUrl = _useNgrok
        ? 'wss://${_hostCtrl.text.trim()}'
        : 'ws://${_localIPCtrl.text.trim()}:8080';
    return _SettingsCard(
      icon: Icons.cable_rounded,
      iconColor: AppColors.primary,
      iconBg: AppColors.primary.withValues(alpha: 0.1),
      title: 'Backend',
      subtitle: 'WebSocket server config',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const SizedBox(height: 4),
          TextField(
            controller: _localIPCtrl,
            decoration: const InputDecoration(
              labelText: 'Local IP (LAN)',
              hintText: 'e.g. 10.213.158.247',
              isDense: true,
            ),
            keyboardType: TextInputType.number,
            style: const TextStyle(fontSize: 13),
            onChanged: (_) => setState(() {}),
          ),
          const SizedBox(height: 10),
          TextField(
            controller: _hostCtrl,
            decoration: const InputDecoration(
              labelText: 'ngrok Host',
              hintText: 'e.g. xxxx.ngrok-free.app',
              isDense: true,
            ),
            style: const TextStyle(fontSize: 13),
            onChanged: (_) => setState(() {}),
          ),
          const SizedBox(height: 10),
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text(
                'Use ngrok tunnel',
                style: TextStyle(
                  fontSize: 13,
                  color: cs.onSurface.withValues(alpha: 0.8),
                ),
              ),
              Switch(
                value: _useNgrok,
                onChanged: (v) => setState(() => _useNgrok = v),
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            'Active URL: $activeUrl',
            style: TextStyle(
              fontSize: 10,
              color: cs.onSurface.withValues(alpha: 0.55),
              fontFamily: 'monospace',
            ),
          ),
          const SizedBox(height: 10),
          SizedBox(
            width: double.infinity,
            child: OutlinedButton(
              onPressed: () {
                widget.notifier.setBackendConfig(
                  ngrokHost: _hostCtrl.text.trim(),
                  useNgrok: _useNgrok,
                  localIP: _localIPCtrl.text.trim(),
                );
                ScaffoldMessenger.of(context).showSnackBar(
                  const SnackBar(
                    content: Text('Backend config saved'),
                    behavior: SnackBarBehavior.floating,
                    duration: Duration(seconds: 2),
                  ),
                );
              },
              child: const Text('Save'),
            ),
          ),
        ],
      ),
    );
  }
}
