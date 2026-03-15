import 'package:flutter/material.dart';

// ─── Brand colours (mirrors the React Tailwind CSS variables) ─────────────────
class AppColors {
  // Primary – indigo/purple matching primary in the web app
  static const primary = Color(0xFF6366F1);
  static const primaryForeground = Color(0xFFFFFFFF);

  // Success – green accent
  static const success = Color(0xFF22C55E);
  static const successForeground = Color(0xFFFFFFFF);

  // Destructive – red
  static const destructive = Color(0xFFEF4444);
  static const destructiveLight = Color(0x1AEF4444); // 10 % opacity

  // Background / surface
  static const backgroundLight = Color(0xFFF8F8FB);
  static const backgroundDark = Color(0xFF0F0F14);

  static const cardLight = Color(0xFFFFFFFF);
  static const cardDark = Color(0xFF1A1A24);

  static const borderLight = Color(0xFFE5E7EB);
  static const borderDark = Color(0xFF2A2A38);

  // Muted
  static const mutedLight = Color(0xFFF3F4F6);
  static const mutedDark = Color(0xFF1E1E2E);

  // Text
  static const foregroundLight = Color(0xFF111827);
  static const foregroundDark = Color(0xFFF9FAFB);
  static const mutedForegroundLight = Color(0xFF6B7280);
  static const mutedForegroundDark = Color(0xFF9CA3AF);

  // Secondary
  static const secondaryLight = Color(0xFFF3F4F6);
  static const secondaryDark = Color(0xFF1E1E2E);
}

// ─── Light theme ─────────────────────────────────────────────────────────────
ThemeData lightTheme = ThemeData(
  useMaterial3: true,
  brightness: Brightness.light,
  colorScheme: ColorScheme.light(
    primary: AppColors.primary,
    onPrimary: AppColors.primaryForeground,
    secondary: AppColors.secondaryLight,
    onSecondary: AppColors.foregroundLight,
    surface: AppColors.cardLight,
    onSurface: AppColors.foregroundLight,
    error: AppColors.destructive,
    onError: Colors.white,
    outline: AppColors.borderLight,
    surfaceContainerHighest: AppColors.mutedLight,
  ),
  scaffoldBackgroundColor: AppColors.backgroundLight,
  cardColor: AppColors.cardLight,
  dividerColor: AppColors.borderLight,
  // Inter is not bundled in pubspec.yaml — removed to prevent font fallback
  // warnings and let Flutter use the system sans-serif (P20 fix).
  textTheme: _buildTextTheme(AppColors.foregroundLight),
  appBarTheme: AppBarTheme(
    backgroundColor: AppColors.cardLight.withValues(alpha: 0.85),
    elevation: 0,
    scrolledUnderElevation: 0.5,
    titleTextStyle: const TextStyle(
      color: AppColors.foregroundLight,
      fontSize: 17,
      fontWeight: FontWeight.w700,
      letterSpacing: -0.3,
    ),
    iconTheme: const IconThemeData(color: AppColors.foregroundLight),
  ),
  navigationBarTheme: NavigationBarThemeData(
    backgroundColor: AppColors.cardLight.withValues(alpha: 0.90),
    indicatorColor: AppColors.primary.withValues(alpha: 0.12),
    labelTextStyle: WidgetStateProperty.resolveWith((states) {
      final active = states.contains(WidgetState.selected);
      return TextStyle(
        fontSize: 11,
        fontWeight: active ? FontWeight.w700 : FontWeight.w500,
        color: active ? AppColors.primary : AppColors.mutedForegroundLight,
      );
    }),
    iconTheme: WidgetStateProperty.resolveWith((states) {
      final active = states.contains(WidgetState.selected);
      return IconThemeData(
        color: active ? AppColors.primary : AppColors.mutedForegroundLight,
        size: 22,
      );
    }),
  ),
  inputDecorationTheme: InputDecorationTheme(
    filled: true,
    fillColor: AppColors.cardLight,
    border: OutlineInputBorder(
      borderRadius: BorderRadius.circular(12),
      borderSide: const BorderSide(color: AppColors.borderLight),
    ),
    enabledBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(12),
      borderSide: const BorderSide(color: AppColors.borderLight),
    ),
    focusedBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(12),
      borderSide: const BorderSide(color: AppColors.primary, width: 2),
    ),
    contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
  ),
  elevatedButtonTheme: ElevatedButtonThemeData(
    style: ElevatedButton.styleFrom(
      backgroundColor: AppColors.primary,
      foregroundColor: Colors.white,
      elevation: 0,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 16),
      textStyle: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
    ),
  ),
  outlinedButtonTheme: OutlinedButtonThemeData(
    style: OutlinedButton.styleFrom(
      foregroundColor: AppColors.foregroundLight,
      side: const BorderSide(color: AppColors.borderLight),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
      padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
      textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
    ),
  ),
  chipTheme: ChipThemeData(
    backgroundColor: AppColors.secondaryLight,
    labelStyle: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600),
    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
  ),
  switchTheme: SwitchThemeData(
    thumbColor: WidgetStateProperty.resolveWith(
      (s) => s.contains(WidgetState.selected) ? Colors.white : Colors.white,
    ),
    trackColor: WidgetStateProperty.resolveWith(
      (s) => s.contains(WidgetState.selected)
          ? AppColors.primary
          : AppColors.borderLight,
    ),
  ),
  sliderTheme: SliderThemeData(
    activeTrackColor: AppColors.primary,
    inactiveTrackColor: AppColors.borderLight,
    thumbColor: AppColors.primary,
    overlayColor: AppColors.primary.withValues(alpha: 0.1),
    trackHeight: 4,
  ),
);

// ─── Dark theme ───────────────────────────────────────────────────────────────
ThemeData darkTheme = ThemeData(
  useMaterial3: true,
  brightness: Brightness.dark,
  colorScheme: ColorScheme.dark(
    primary: AppColors.primary,
    onPrimary: AppColors.primaryForeground,
    secondary: AppColors.secondaryDark,
    onSecondary: AppColors.foregroundDark,
    surface: AppColors.cardDark,
    onSurface: AppColors.foregroundDark,
    error: AppColors.destructive,
    onError: Colors.white,
    outline: AppColors.borderDark,
    surfaceContainerHighest: AppColors.mutedDark,
  ),
  scaffoldBackgroundColor: AppColors.backgroundDark,
  cardColor: AppColors.cardDark,
  dividerColor: AppColors.borderDark,
  textTheme: _buildTextTheme(AppColors.foregroundDark),
  appBarTheme: AppBarTheme(
    backgroundColor: AppColors.cardDark.withValues(alpha: 0.85),
    elevation: 0,
    scrolledUnderElevation: 0.5,
    titleTextStyle: const TextStyle(
      color: AppColors.foregroundDark,
      fontSize: 17,
      fontWeight: FontWeight.w700,
      letterSpacing: -0.3,
    ),
    iconTheme: const IconThemeData(color: AppColors.foregroundDark),
  ),
  navigationBarTheme: NavigationBarThemeData(
    backgroundColor: AppColors.cardDark.withValues(alpha: 0.90),
    indicatorColor: AppColors.primary.withValues(alpha: 0.15),
    labelTextStyle: WidgetStateProperty.resolveWith((states) {
      final active = states.contains(WidgetState.selected);
      return TextStyle(
        fontSize: 11,
        fontWeight: active ? FontWeight.w700 : FontWeight.w500,
        color: active ? AppColors.primary : AppColors.mutedForegroundDark,
      );
    }),
    iconTheme: WidgetStateProperty.resolveWith((states) {
      final active = states.contains(WidgetState.selected);
      return IconThemeData(
        color: active ? AppColors.primary : AppColors.mutedForegroundDark,
        size: 22,
      );
    }),
  ),
  inputDecorationTheme: InputDecorationTheme(
    filled: true,
    fillColor: AppColors.cardDark,
    border: OutlineInputBorder(
      borderRadius: BorderRadius.circular(12),
      borderSide: const BorderSide(color: AppColors.borderDark),
    ),
    enabledBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(12),
      borderSide: const BorderSide(color: AppColors.borderDark),
    ),
    focusedBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(12),
      borderSide: const BorderSide(color: AppColors.primary, width: 2),
    ),
    contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
  ),
  elevatedButtonTheme: ElevatedButtonThemeData(
    style: ElevatedButton.styleFrom(
      backgroundColor: AppColors.primary,
      foregroundColor: Colors.white,
      elevation: 0,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 16),
      textStyle: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
    ),
  ),
  outlinedButtonTheme: OutlinedButtonThemeData(
    style: OutlinedButton.styleFrom(
      foregroundColor: AppColors.foregroundDark,
      side: const BorderSide(color: AppColors.borderDark),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
      padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
      textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
    ),
  ),
  chipTheme: ChipThemeData(
    backgroundColor: AppColors.secondaryDark,
    labelStyle: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600),
    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
  ),
  switchTheme: SwitchThemeData(
    thumbColor: WidgetStateProperty.resolveWith(
      (s) => s.contains(WidgetState.selected) ? Colors.white : Colors.white,
    ),
    trackColor: WidgetStateProperty.resolveWith(
      (s) => s.contains(WidgetState.selected)
          ? AppColors.primary
          : AppColors.borderDark,
    ),
  ),
  sliderTheme: SliderThemeData(
    activeTrackColor: AppColors.primary,
    inactiveTrackColor: AppColors.borderDark,
    thumbColor: AppColors.primary,
    overlayColor: AppColors.primary.withValues(alpha: 0.1),
    trackHeight: 4,
  ),
);

TextTheme _buildTextTheme(Color baseColor) {
  return TextTheme(
    displayLarge: TextStyle(
      fontSize: 32,
      fontWeight: FontWeight.w800,
      letterSpacing: -1,
      color: baseColor,
    ),
    headlineMedium: TextStyle(
      fontSize: 24,
      fontWeight: FontWeight.w800,
      letterSpacing: -0.5,
      color: baseColor,
    ),
    titleLarge: TextStyle(
      fontSize: 18,
      fontWeight: FontWeight.w700,
      color: baseColor,
    ),
    titleMedium: TextStyle(
      fontSize: 15,
      fontWeight: FontWeight.w600,
      color: baseColor,
    ),
    titleSmall: TextStyle(
      fontSize: 13,
      fontWeight: FontWeight.w600,
      color: baseColor,
    ),
    bodyLarge: TextStyle(fontSize: 15, color: baseColor),
    bodyMedium: TextStyle(fontSize: 13, color: baseColor),
    bodySmall:
        TextStyle(fontSize: 11, color: baseColor.withValues(alpha: 0.65)),
    labelSmall: TextStyle(
      fontSize: 10,
      fontWeight: FontWeight.w600,
      letterSpacing: 0.5,
      color: baseColor.withValues(alpha: 0.65),
    ),
  );
}
