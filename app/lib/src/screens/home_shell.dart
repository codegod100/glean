import 'package:flutter/material.dart';

import '../api/client.dart';
import '../app_state.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/common.dart';
import 'articles_screen.dart';

/// Articles, with the feed list in a drawer.
///
/// One screen rather than a tab bar: the reader has exactly one thing to
/// look at, and choosing a feed is navigation within it rather than a
/// separate destination.
class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  Feed _selected = Feed.all(0);
  bool _refreshing = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      AppScope.read(context).load();
    });
  }

  Future<void> _addFeed() async {
    final url = await showDialog<String>(
      context: context,
      builder: (_) => const _AddFeedDialog(),
    );
    if (url == null || url.isEmpty || !mounted) return;
    try {
      await AppScope.read(context).addFeed(url);
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  Future<void> _refresh() async {
    setState(() => _refreshing = true);
    try {
      final result = await AppScope.read(context).refresh();
      if (!mounted) return;
      showToast(
        context,
        result.errors.isEmpty
            ? '${result.added} new'
            : '${result.added} new, ${result.errors.length} feed(s) failed',
      );
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    } finally {
      if (mounted) setState(() => _refreshing = false);
    }
  }

  Future<void> _markAllRead() async {
    final scope = _selected.isAll ? 'everything' : _selected.title;
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        shape: const RoundedRectangleBorder(),
        title: Text('Mark $scope read?'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Cancel')),
          TextButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Mark read')),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    await AppScope.read(context).markAllRead(feedUrl: _selected.feedUrl);
  }

  Future<void> _remove(Feed f) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        shape: const RoundedRectangleBorder(),
        title: const Text('Unsubscribe?'),
        content: Text(f.title),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Cancel')),
          TextButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Unsubscribe')),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    await AppScope.read(context).removeFeed(f.feedUrl);
    if (mounted && _selected.feedUrl == f.feedUrl) {
      setState(() => _selected = Feed.all(0));
    }
  }

  @override
  Widget build(BuildContext context) {
    final app = AppScope.of(context);
    final c = GleanColors.of(context);

    return Scaffold(
      appBar: AppBar(
        title: Text(_selected.isAll ? 'glean' : _selected.title),
        actions: [
          IconButton(
            tooltip: 'Mark all read',
            icon: const Icon(Icons.done_all),
            onPressed: _markAllRead,
          ),
          IconButton(
            tooltip: 'Refresh',
            icon: _refreshing
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2))
                : const Icon(Icons.refresh),
            onPressed: _refreshing ? null : _refresh,
          ),
        ],
      ),
      drawer: Drawer(
        backgroundColor: c.bg,
        child: SafeArea(
          child: Column(
            children: [
              ListTile(
                title: Text('Feeds',
                    style: Theme.of(context).textTheme.titleMedium),
                trailing: IconButton(
                  icon: const Icon(Icons.add),
                  tooltip: 'Add feed',
                  onPressed: () {
                    Navigator.of(context).pop();
                    _addFeed();
                  },
                ),
              ),
              if (app.error != null)
                Padding(
                  padding: const EdgeInsets.all(16),
                  child: Text(app.error!,
                      style: Theme.of(context)
                          .textTheme
                          .bodySmall
                          ?.copyWith(color: c.danger)),
                ),
              Expanded(
                child: ListView(
                  children: [
                    for (final f in app.sidebar)
                      ListTile(
                        selected: f.feedUrl == _selected.feedUrl,
                        title: Text(f.title,
                            maxLines: 1, overflow: TextOverflow.ellipsis),
                        trailing: f.unread > 0
                            ? GleanTag('${f.unread}', emphasis: !f.isAll)
                            : null,
                        onLongPress: f.isAll ? null : () => _remove(f),
                        onTap: () {
                          setState(() => _selected = f);
                          Navigator.of(context).pop();
                        },
                      ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
      body: ArticlesScreen(key: ValueKey(_selected.feedUrl), feed: _selected),
    );
  }
}

class _AddFeedDialog extends StatefulWidget {
  const _AddFeedDialog();

  @override
  State<_AddFeedDialog> createState() => _AddFeedDialogState();
}

class _AddFeedDialogState extends State<_AddFeedDialog> {
  final _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      shape: const RoundedRectangleBorder(),
      title: const Text('Add feed'),
      content: TextField(
        controller: _controller,
        autofocus: true,
        keyboardType: TextInputType.url,
        decoration: const InputDecoration(hintText: 'https://example.com/feed.xml'),
        onSubmitted: (v) => Navigator.pop(context, v.trim()),
      ),
      actions: [
        TextButton(
            onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        TextButton(
          onPressed: () => Navigator.pop(context, _controller.text.trim()),
          child: const Text('Add'),
        ),
      ],
    );
  }
}
