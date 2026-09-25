import 'package:flutter/material.dart';

/// Flutter port of web/src/app.css.
///
/// Pulseboard's web design system is deliberately brutalist: a monochrome ink/paper
/// base with one accent green, sharp corners, 2px borders and hard offset
/// shadows, monospace throughout. Keeping that identity matters more than
/// looking like stock Material, so this leans on a shared token set rather
/// than ColorScheme.fromSeed.
class PulseboardColors extends ThemeExtension<PulseboardColors> {
  const PulseboardColors({
    required this.bg,
    required this.fg,
    required this.surface,
    required this.border,
    required this.muted,
    required this.faint,
    required this.accent,
    required this.accentInk,
    required this.danger,
  });

  final Color bg;
  final Color fg;
  final Color surface;
  final Color border;
  final Color muted;
  final Color faint;
  final Color accent;
  final Color accentInk;
  final Color danger;

  static const light = PulseboardColors(
    bg: Color(0xFFFAFAF7),
    fg: Color(0xFF0A0A0A),
    surface: Color(0xFFF0EFE9),
    border: Color(0xFF0A0A0A),
    muted: Color(0xFF6B6B6B),
    faint: Color(0xFFC8C8C2),
    accent: Color(0xFF00754A),
    accentInk: Color(0xFFECFFF4),
    danger: Color(0xFFC82014),
  );

  static const dark = PulseboardColors(
    bg: Color(0xFF0A0A0A),
    fg: Color(0xFFF5F5EF),
    surface: Color(0xFF161616),
    border: Color(0xFFF5F5EF),
    muted: Color(0xFF9A9A9A),
    faint: Color(0xFF3A3A3A),
    accent: Color(0xFF00754A),
    accentInk: Color(0xFF062018),
    danger: Color(0xFFFF5A4D),
  );

  static PulseboardColors of(BuildContext context) =>
      Theme.of(context).extension<PulseboardColors>()!;

  @override
  PulseboardColors copyWith() => this;

  @override
  PulseboardColors lerp(ThemeExtension<PulseboardColors>? other, double t) =>
      t < 0.5 ? this : (other as PulseboardColors? ?? this);
}

/// Compact UI labels retain the app's technical voice.
const kMonoFallback = <String>['JetBrains Mono', 'IBM Plex Mono', 'monospace'];

/// Reading text is deliberately a separate voice: a classic serif, with the
/// looser leading of a paper viewer. This is what lets a long article title or
/// excerpt feel like something to read rather than another UI control.
const kEditorialFallback = <String>[
  'Noto Serif',
  'Georgia',
  'Times New Roman',
  'serif',
];

ThemeData pulseboardTheme(Brightness brightness) {
  final c = brightness == Brightness.dark
      ? PulseboardColors.dark
      : PulseboardColors.light;

  TextStyle mono(
    double size, {
    FontWeight weight = FontWeight.w400,
    Color? color,
  }) => TextStyle(
    fontFamilyFallback: kMonoFallback,
    fontSize: size,
    fontWeight: weight,
    color: color ?? c.fg,
    height: 1.45,
  );

  TextStyle editorial(
    double size, {
    FontWeight weight = FontWeight.w400,
    Color? color,
    FontStyle? fontStyle,
  }) => TextStyle(
    fontFamilyFallback: kEditorialFallback,
    fontSize: size,
    fontWeight: weight,
    color: color ?? c.fg,
    fontStyle: fontStyle,
    height: 1.58,
  );

  return ThemeData(
    brightness: brightness,
    scaffoldBackgroundColor: c.bg,
    canvasColor: c.bg,
    dividerColor: c.border,
    splashFactory: NoSplash.splashFactory,
    colorScheme: ColorScheme.fromSeed(
      seedColor: c.accent,
      brightness: brightness,
    ).copyWith(surface: c.bg, primary: c.accent, error: c.danger),
    extensions: [c],
    textTheme: TextTheme(
      displaySmall: editorial(30, weight: FontWeight.w600),
      headlineSmall: editorial(23, weight: FontWeight.w600),
      titleMedium: mono(16, weight: FontWeight.w700),
      bodyLarge: editorial(19, weight: FontWeight.w500),
      bodyMedium: editorial(16),
      bodySmall: mono(12, color: c.muted),
      labelLarge: mono(
        12,
        weight: FontWeight.w700,
      ).copyWith(letterSpacing: 0.35),
    ),
    appBarTheme: AppBarTheme(
      backgroundColor: c.bg,
      foregroundColor: c.fg,
      elevation: 0,
      centerTitle: false,
      shape: Border(bottom: BorderSide(color: c.border, width: 2)),
      titleTextStyle: mono(18, weight: FontWeight.w700),
    ),
    iconTheme: IconThemeData(color: c.fg, size: 20),
    visualDensity: VisualDensity.standard,
    progressIndicatorTheme: ProgressIndicatorThemeData(color: c.accent),
    drawerTheme: DrawerThemeData(
      backgroundColor: c.bg,
      shape: Border(right: BorderSide(color: c.border, width: 2)),
    ),
    listTileTheme: ListTileThemeData(
      contentPadding: const EdgeInsets.symmetric(horizontal: 16),
      minVerticalPadding: 10,
      selectedColor: c.fg,
      selectedTileColor: c.surface,
      titleTextStyle: mono(14, weight: FontWeight.w600),
      subtitleTextStyle: mono(12, color: c.muted),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: c.accent,
        foregroundColor: c.accentInk,
        disabledBackgroundColor: c.faint,
        disabledForegroundColor: c.muted,
        shape: const RoundedRectangleBorder(),
        padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 16),
        textStyle: mono(13, weight: FontWeight.w700),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: c.fg,
        side: BorderSide(color: c.border, width: 2),
        shape: const RoundedRectangleBorder(),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        textStyle: mono(13, weight: FontWeight.w700),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: c.fg,
        shape: const RoundedRectangleBorder(),
        textStyle: mono(13, weight: FontWeight.w700),
      ),
    ),
    snackBarTheme: SnackBarThemeData(
      backgroundColor: c.fg,
      contentTextStyle: mono(13, color: c.bg),
      behavior: SnackBarBehavior.floating,
      shape: const RoundedRectangleBorder(),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: c.surface,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.zero,
        borderSide: BorderSide(color: c.border, width: 2),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.zero,
        borderSide: BorderSide(color: c.border, width: 2),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.zero,
        borderSide: BorderSide(color: c.accent, width: 2),
      ),
      hintStyle: mono(14, color: c.muted),
      contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 13),
    ),
  );
}
