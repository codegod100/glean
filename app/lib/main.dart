import 'package:flutter/material.dart';

import 'src/app_state.dart';
import 'src/screens/home_shell.dart';
import 'src/theme.dart';

void main() {
  runApp(const GleanApp());
}

class GleanApp extends StatefulWidget {
  const GleanApp({super.key});

  @override
  State<GleanApp> createState() => _GleanAppState();
}

class _GleanAppState extends State<GleanApp> {
  final _state = AppState();

  @override
  void initState() {
    super.initState();
    // The feed list loads from the shell, which is the only screen that
    // needs it; there is no session to restore first.
  }

  @override
  void dispose() {
    _state.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AppScope(
      state: _state,
      child: MaterialApp(
        title: 'glean',
        debugShowCheckedModeBanner: false,
        theme: gleanTheme(Brightness.light),
        darkTheme: gleanTheme(Brightness.dark),
        home: const HomeShell(),
      ),
    );
  }
}


