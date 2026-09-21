import 'package:flutter/material.dart';

import '../app_state.dart';
import '../theme.dart';
import 'articles_screen.dart';
import 'dashboard_screen.dart';
import 'login_screen.dart';
import 'trending_screen.dart';

/// Bottom-tab shell. Trending is public; the rest require a session, so a
/// signed-out visitor sees Trending with a prompt to sign in rather than a
/// wall of failed requests.
class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  int _index = 0;

  @override
  Widget build(BuildContext context) {
    final app = AppScope.of(context);
    final c = GleanColors.of(context);

    // Tab set depends on the session: no point offering Home to a visitor who
    // would only get a 401.
    final tabs = <_Tab>[
      if (app.signedIn)
        const _Tab(icon: Icons.home_outlined, label: 'Home', child: DashboardScreen()),
      if (app.signedIn)
        const _Tab(icon: Icons.article_outlined, label: 'Articles', child: ArticlesScreen()),
      const _Tab(icon: Icons.trending_up, label: 'Trending', child: TrendingScreen()),
    ];
    final index = _index.clamp(0, tabs.length - 1);

    return Scaffold(
      appBar: AppBar(
        title: Text(tabs[index].label == 'Home' ? 'glean' : tabs[index].label),
        actions: [
          if (app.signedIn)
            IconButton(
              tooltip: 'Sign out',
              icon: const Icon(Icons.logout),
              onPressed: () => AppScope.read(context).signOut(),
            )
          else
            TextButton(
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute(builder: (_) => const LoginScreen()),
              ),
              child: const Text('Sign in'),
            ),
        ],
      ),
      // ArticlesScreen builds its own Scaffold/AppBar for its filter bar, so it
      // is shown without this shell's chrome when selected.
      body: IndexedStack(
        index: index,
        children: [for (final t in tabs) t.child],
      ),
      bottomNavigationBar: tabs.length < 2
          ? null
          : Container(
              decoration: BoxDecoration(
                border: Border(top: BorderSide(color: c.border, width: 2)),
              ),
              child: NavigationBar(
                selectedIndex: index,
                backgroundColor: c.bg,
                indicatorColor: c.accent,
                onDestinationSelected: (i) => setState(() => _index = i),
                destinations: [
                  for (final t in tabs)
                    NavigationDestination(icon: Icon(t.icon), label: t.label),
                ],
              ),
            ),
    );
  }
}

class _Tab {
  const _Tab({required this.icon, required this.label, required this.child});

  final IconData icon;
  final String label;
  final Widget child;
}
