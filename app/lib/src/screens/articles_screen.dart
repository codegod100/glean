import 'package:flutter/material.dart';

import '../api/client.dart';
import '../api/responses.dart';
import '../app_state.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/article_tile.dart';
import '../widgets/common.dart';
import 'article_screen.dart';

/// The main reading list: /api/articles with the same filters the web app
/// exposes (status, search, category, sort, per-feed).
class ArticlesScreen extends StatefulWidget {
  const ArticlesScreen({super.key, this.feedUrl, this.title});

  /// When set, the list is scoped to one feed.
  final String? feedUrl;
  final String? title;

  @override
  State<ArticlesScreen> createState() => _ArticlesScreenState();
}

class _ArticlesScreenState extends State<ArticlesScreen> {
  Future<ArticlesResponse>? _future;
  final _searchController = TextEditingController();

  int _page = 1;
  String _status = 'unread';
  String? _category;
  String _search = '';
  bool _sortOldest = false;

  /// Local overrides for rows the user has just acted on, so a like or a
  /// mark-read is reflected immediately without refetching the page.
  final Map<int, Article> _patched = {};

  @override
  void initState() {
    super.initState();
    _load();
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
            page: _page,
            feedUrl: widget.feedUrl,
            status: _status,
            search: _search.isEmpty ? null : _search,
            category: _category,
            sortOldest: _sortOldest,
          );
    });
  }

  void _goToPage(int page) {
    _page = page;
    _load();
  }

  Future<void> _toggleRead(Article a) async {
    final client = AppScope.read(context).client;
    // Optimistic: the row flips now, and reverts if the server disagrees.
    setState(() => _patched[a.id] = a.copyWith(isRead: !a.isRead));
    try {
      final isRead = a.isRead ? await client.markUnread(a.id) : await client.markRead(a.id);
      if (mounted) setState(() => _patched[a.id] = a.copyWith(isRead: isRead));
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _patched[a.id] = a);
      showToast(context, e.message);
    }
  }

  Future<void> _toggleLike(Article a) async {
    final client = AppScope.read(context).client;
    setState(() => _patched[a.id] = a.copyWith(
          hasLiked: !a.hasLiked,
          likeCount: a.likeCount + (a.hasLiked ? -1 : 1),
        ));
    try {
      final r = await client.likeArticle(a.id);
      if (mounted) {
        setState(() => _patched[a.id] = a.copyWith(hasLiked: r.liked, likeCount: r.likeCount));
      }
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _patched[a.id] = a);
      showToast(context, e.message);
    }
  }

  Future<void> _markAllRead() async {
    final messenger = ScaffoldMessenger.of(context);
    try {
      await AppScope.read(context).client.markAllRead(
            feedUrl: widget.feedUrl,
            category: _category,
          );
      _load();
    } on ApiException catch (e) {
      messenger.showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(widget.title ?? 'Articles'),
        actions: [
          IconButton(
            tooltip: 'Mark all read',
            icon: const Icon(Icons.done_all),
            onPressed: _markAllRead,
          ),
          IconButton(
            tooltip: _sortOldest ? 'Newest first' : 'Oldest first',
            icon: Icon(_sortOldest ? Icons.arrow_upward : Icons.arrow_downward),
            onPressed: () {
              _sortOldest = !_sortOldest;
              _goToPage(1);
            },
          ),
        ],
      ),
      body: Column(
        children: [
          _SearchBar(
            controller: _searchController,
            onSubmit: (q) {
              _search = q;
              _goToPage(1);
            },
          ),
          _StatusFilter(
            status: _status,
            onChanged: (s) {
              _status = s;
              _goToPage(1);
            },
          ),
          Expanded(
            child: AsyncView<ArticlesResponse>(
              future: _future,
              onRetry: _load,
              builder: (context, data) => _list(context, data),
            ),
          ),
        ],
      ),
    );
  }

  Widget _list(BuildContext context, ArticlesResponse data) {
    if (data.articles.isEmpty) {
      return const EmptyView(message: 'Nothing here.\nTry a different filter.');
    }
    return RefreshIndicator(
      onRefresh: () async => _load(),
      child: ListView.builder(
        itemCount: data.articles.length + 1,
        itemBuilder: (context, i) {
          if (i == data.articles.length) {
            return _Pager(pagination: data.pagination, onPage: _goToPage);
          }
          final raw = data.articles[i];
          final a = _patched[raw.id] ?? raw;
          return ArticleTile(
            article: a,
            expanded: data.expandedView,
            onToggleRead: () => _toggleRead(a),
            onToggleLike: () => _toggleLike(a),
            onTap: () async {
              await Navigator.of(context).push(
                MaterialPageRoute(builder: (_) => ArticleScreen(articleId: a.id)),
              );
              // Reading marks it read server-side; reflect that on return.
              if (mounted) setState(() => _patched[a.id] = a.copyWith(isRead: true));
            },
          );
        },
      ),
    );
  }
}

class _SearchBar extends StatelessWidget {
  const _SearchBar({required this.controller, required this.onSubmit});

  final TextEditingController controller;
  final ValueChanged<String> onSubmit;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 12, 12, 8),
      child: TextField(
        controller: controller,
        textInputAction: TextInputAction.search,
        decoration: InputDecoration(
          hintText: 'Search articles',
          isDense: true,
          prefixIcon: const Icon(Icons.search, size: 18),
          suffixIcon: controller.text.isEmpty
              ? null
              : IconButton(
                  icon: const Icon(Icons.clear, size: 18),
                  onPressed: () {
                    controller.clear();
                    onSubmit('');
                  },
                ),
        ),
        onSubmitted: onSubmit,
      ),
    );
  }
}

class _StatusFilter extends StatelessWidget {
  const _StatusFilter({required this.status, required this.onChanged});

  final String status;
  final ValueChanged<String> onChanged;

  // Mirrors the status values handleArticles accepts.
  static const _options = {'unread': 'Unread', 'all': 'All', 'read': 'Read'};

  @override
  Widget build(BuildContext context) {
    final c = GleanColors.of(context);
    return SizedBox(
      height: 40,
      child: ListView(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        children: [
          for (final e in _options.entries)
            Padding(
              padding: const EdgeInsets.only(right: 8),
              child: GestureDetector(
                onTap: () => onChanged(e.key),
                child: Container(
                  alignment: Alignment.center,
                  padding: const EdgeInsets.symmetric(horizontal: 14),
                  decoration: BoxDecoration(
                    color: status == e.key ? c.accent : Colors.transparent,
                    border: Border.all(color: c.border, width: 2),
                  ),
                  child: Text(
                    e.value,
                    style: Theme.of(context).textTheme.labelLarge?.copyWith(
                          color: status == e.key ? c.accentInk : c.fg,
                        ),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _Pager extends StatelessWidget {
  const _Pager({required this.pagination, required this.onPage});

  final Pagination pagination;
  final ValueChanged<int> onPage;

  @override
  Widget build(BuildContext context) {
    if (!pagination.hasPrev && !pagination.hasNext) return const SizedBox(height: 24);
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          GleanButton(
            label: 'Prev',
            onPressed: pagination.hasPrev ? () => onPage(pagination.prevPage) : null,
          ),
          Text('page ${pagination.page}', style: Theme.of(context).textTheme.bodySmall),
          GleanButton(
            label: 'Next',
            onPressed: pagination.hasNext ? () => onPage(pagination.nextPage) : null,
          ),
        ],
      ),
    );
  }
}
