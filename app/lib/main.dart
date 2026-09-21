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
    _state.bootstrap();
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
        home: const _Root(),
      ),
    );
  }
}

/// Holds the splash until the persisted session has been checked, so the app
/// does not flash the signed-out shell at a signed-in user.
class _Root extends StatelessWidget {
  const _Root();

  @override
  Widget build(BuildContext context) {
    final app = AppScope.of(context);
    if (app.loading && app.user == null) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    return const HomeShell();
  }
}
