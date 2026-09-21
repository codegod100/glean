import 'package:flutter/material.dart';

/// Flutter port of web/src/app.css.
///
/// Glean's web design system is deliberately brutalist: a monochrome ink/paper
/// base with one accent green, sharp corners, 2px borders and hard offset
/// shadows, monospace throughout. Keeping that identity matters more than
/// looking like stock Material, so this leans on a shared token set rather
/// than ColorScheme.fromSeed.
class GleanColors extends ThemeExtension<GleanColors> {
  const GleanColors({
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

  static const light = GleanColors(
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

  static const dark = GleanColors(
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

  static GleanColors of(BuildContext context) =>
      Theme.of(context).extension<GleanColors>()!;

  @override
  GleanColors copyWith() => this;

  @override
  GleanColors lerp(ThemeExtension<GleanColors>? other, double t) =>
      t < 0.5 ? this : (other as GleanColors? ?? this);
}

/// The web app asks for JetBrains Mono and falls back through IBM Plex Mono to
/// the platform monospace. Doing the same here keeps the look without a
/// runtime font download; bundling the TTF would pin it exactly.
const kMonoFallback = <String>['JetBrains Mono', 'IBM Plex Mono', 'monospace'];

ThemeData gleanTheme(Brightness brightness) {
  final c = brightness == Brightness.dark ? GleanColors.dark : GleanColors.light;

  TextStyle mono(double size, {FontWeight weight = FontWeight.w400, Color? color}) =>
      TextStyle(
        fontFamilyFallback: kMonoFallback,
        fontSize: size,
        fontWeight: weight,
        color: color ?? c.fg,
        height: 1.45,
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
      displaySmall: mono(28, weight: FontWeight.w700),
      headlineSmall: mono(20, weight: FontWeight.w700),
      titleMedium: mono(16, weight: FontWeight.w700),
      bodyLarge: mono(15),
      bodyMedium: mono(14),
      bodySmall: mono(12, color: c.muted),
      labelLarge: mono(13, weight: FontWeight.w700),
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
    ),
  );
}
