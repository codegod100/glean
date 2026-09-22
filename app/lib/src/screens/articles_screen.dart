import 'package:flutter/material.dart';

import '../api/client.dart';
import '../app_state.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/article_tile.dart';
import '../widgets/common.dart';
import 'article_screen.dart';

/// The reading list, scoped by the feed selected in the drawer.
class ArticlesScreen extends StatefulWidget {
  const ArticlesScreen({super.key, required this.feed});

  final Feed feed;

  @override
  State<ArticlesScreen> createState() => _ArticlesScreenState();
}

class _ArticlesScreenState extends State<ArticlesScreen> {
  Future<List<Article>>? _future;
  final _searchController = TextEditingController();

  String _status = 'unread';
  String _search = '';

  /// Rows the reader has just acted on, so a tap reflects immediately
  /// without refetching the list.
  final Map<int, Article> _patched = {};

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void didUpdateWidget(ArticlesScreen old) {
    super.didUpdateWidget(old);
    if (old.feed.feedUrl != widget.feed.feedUrl) _load();
  }

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  void _load() {
    setState(() {
      _patched.clear();
      _future = AppScope.read(context).client.articles(
            status: _status,
            feedUrl: widget.feed.feedUrl,
            search: _search,
          );
    });
  }

  Future<void> _toggleRead(Article a) async {
    final app = AppScope.read(context);
    setState(() => _patched[a.id] = a.copyWith(isRead: !a.isRead));
    app.adjustUnread(a.isRead ? 1 : -1);
    try {
      await app.client.setRead(a.id, read: !a.isRead);
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _patched[a.id] = a);
      app.adjustUnread(a.isRead ? -1 : 1);
      showToast(context, e.message);
    }
  }

  Future<void> _open(Article a) async {
    // Read the state up front: after the await the element may be gone, and
    // a lint about context across an async gap is pointing at a real one.
    final app = AppScope.read(context);
    await Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => ArticleScreen(articleId: a.id)),
    );
    if (!mounted || a.isRead) return;
    // Opening marks it read server-side.
    setState(() => _patched[a.id] = a.copyWith(isRead: true));
    app.adjustUnread(-1);
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(12, 12, 12, 6),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _searchController,
                  textInputAction: TextInputAction.search,
                  decoration: const InputDecoration(
                    hintText: 'Search',
                    isDense: true,
                    prefixIcon: Icon(Icons.search, size: 18),
                  ),
                  onSubmitted: (v) {
                    _search = v.trim();
                    _load();
                  },
                ),
              ),
              const SizedBox(width: 8),
              // Search spans everything, so the status filter is meaningless
              // while one is active.
              if (_search.isEmpty)
                _StatusFilter(
                  status: _status,
                  onChanged: (s) {
                    _status = s;
                    _load();
                  },
                ),
            ],
          ),
        ),
        Expanded(
          child: AsyncView<List<Article>>(
            future: _future,
            onRetry: _load,
            builder: (context, items) {
              if (items.isEmpty) {
                return EmptyView(
                  message: _search.isNotEmpty
                      ? 'Nothing matches "$_search".'
                      : 'Nothing here.',
                );
              }
              return RefreshIndicator(
                onRefresh: () async => _load(),
                child: ListView.builder(
                  itemCount: items.length,
                  itemBuilder: (context, i) {
                    final a = _patched[items[i].id] ?? items[i];
                    return ArticleTile(
                      article: a,
                      onToggleRead: () => _toggleRead(a),
                      onTap: () => _open(a),
                    );
                  },
                ),
              );
            },
          ),
        ),
      ],
    );
  }
}

class _StatusFilter extends StatelessWidget {
  const _StatusFilter({required this.status, required this.onChanged});

  final String status;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    final c = GleanColors.of(context);
    return DropdownButton<String>(
      value: status,
      underline: const SizedBox.shrink(),
      style: Theme.of(context).textTheme.labelLarge?.copyWith(color: c.fg),
      onChanged: (v) => onChanged(v ?? 'unread'),
      items: const [
        DropdownMenuItem(value: 'unread', child: Text('Unread')),
        DropdownMenuItem(value: 'all', child: Text('All')),
        DropdownMenuItem(value: 'read', child: Text('Read')),
      ],
    );
  }
}
