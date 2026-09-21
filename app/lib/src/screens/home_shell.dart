import 'package:flutter/material.dart';

import '../app_state.dart';
import '../theme.dart';
import 'articles_screen.dart';
import 'dashboard_screen.dart';
import 'discover_screen.dart';
import 'feeds_screen.dart';
import 'library_screen.dart';
import 'login_screen.dart';
import 'profile_screen.dart';
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
      if (app.signedIn) ...[
        const _Tab(icon: Icons.home_outlined, label: 'Home', child: DashboardScreen()),
        const _Tab(icon: Icons.article_outlined, label: 'Articles', child: ArticlesScreen()),
        const _Tab(icon: Icons.rss_feed, label: 'Feeds', child: FeedsScreen()),
        const _Tab(icon: Icons.bookmark_border, label: 'Library', child: LibraryScreen()),
        const _Tab(icon: Icons.explore_outlined, label: 'Discover', child: DiscoverScreen()),
      ] else
        const _Tab(icon: Icons.trending_up, label: 'Trending', child: TrendingScreen()),
    ];
    final index = _index.clamp(0, tabs.length - 1);

    return Scaffold(
      appBar: AppBar(
        title: Text(tabs[index].label == 'Home' ? 'glean' : tabs[index].label),
        actions: [
          if (app.signedIn) ...[
            IconButton(
              tooltip: 'Trending',
              icon: const Icon(Icons.trending_up),
              onPressed: () => Navigator.of(context).push(MaterialPageRoute(
                builder: (_) => Scaffold(
                  appBar: AppBar(title: const Text('Trending')),
                  body: const TrendingScreen(),
                ),
              )),
            ),
            IconButton(
              tooltip: 'Profile',
              icon: const Icon(Icons.person_outline),
              onPressed: () => Navigator.of(context).push(MaterialPageRoute(
                builder: (_) => Scaffold(
                  appBar: AppBar(title: const Text('Profile')),
                  body: const ProfileScreen(),
                ),
              )),
            ),
            IconButton(
              tooltip: 'Sign out',
              icon: const Icon(Icons.logout),
              onPressed: () => AppScope.read(context).signOut(),
            ),
          ]
          else
            TextButton(
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute(builder: (_) => const LoginScreen()),
              ),
              child: const Text('Sign in'),
            ),
        ],
      ),
      body: _LazyIndexedStack(index: index, children: [for (final t in tabs) t.child]),
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


/// IndexedStack keeps every tab alive, which is what we want -- scroll position
/// and loaded pages survive switching -- but it also *builds* them all up
/// front, so every screen would fire its initial fetch at startup whether or
/// not the reader ever opens it. This builds each tab on first visit and keeps
/// it alive from then on.
class _LazyIndexedStack extends StatefulWidget {
  const _LazyIndexedStack({required this.index, required this.children});

  final int index;
  final List<Widget> children;

  @override
  State<_LazyIndexedStack> createState() => _LazyIndexedStackState();
}

class _LazyIndexedStackState extends State<_LazyIndexedStack> {
  final _visited = <int>{};

  @override
  void initState() {
    super.initState();
    _visited.add(widget.index);
  }

  @override
  void didUpdateWidget(_LazyIndexedStack old) {
    super.didUpdateWidget(old);
    _visited.add(widget.index);
  }

  @override
  Widget build(BuildContext context) {
    return IndexedStack(
      index: widget.index,
      children: [
        for (var i = 0; i < widget.children.length; i++)
          if (_visited.contains(i)) widget.children[i] else const SizedBox.shrink(),
      ],
    );
  }
}
