import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../api/client.dart';
import '../api/responses.dart';
import '../app_state.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/common.dart';
import 'articles_screen.dart';

/// Subscription management: the list, categories, dead feeds, and the
/// add/edit/remove/refresh/OPML actions from /api/feeds.
class FeedsScreen extends StatefulWidget {
  const FeedsScreen({super.key});

  @override
  State<FeedsScreen> createState() => _FeedsScreenState();
}

class _FeedsScreenState extends State<FeedsScreen> {
  Future<FeedsResponse>? _future;
  int _page = 1;
  String? _category;
  bool _refreshing = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _load() {
    setState(() {
      _future = AppScope.read(context).client.feeds(page: _page, category: _category);
    });
  }

  Future<void> _addFeed() async {
    final url = await showDialog<String>(
      context: context,
      builder: (_) => const _AddFeedDialog(),
    );
    if (url == null || url.isEmpty || !mounted) return;
    try {
      await AppScope.read(context).client.addFeed(url);
      if (!mounted) return;
      showToast(context, 'Subscribed.');
      _load();
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  Future<void> _remove(Subscription s) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        shape: const RoundedRectangleBorder(),
        title: const Text('Unsubscribe?'),
        content: Text(s.feedTitle),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
          TextButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Unsubscribe')),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    try {
      await AppScope.read(context).client.removeFeed(s.feedUrl);
      if (!mounted) return;
      _load();
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  Future<void> _editCategory(Subscription s) async {
    final category = await showDialog<String>(
      context: context,
      builder: (_) => _EditCategoryDialog(initial: s.category),
    );
    if (category == null || !mounted) return;
    try {
      await AppScope.read(context).client.editFeed(s.feedUrl, category: category);
      if (!mounted) return;
      _load();
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  Future<void> _refreshAll() async {
    setState(() => _refreshing = true);
    try {
      await AppScope.read(context).client.refreshFeeds();
      if (!mounted) return;
      showToast(context, 'Refresh queued.');
      _load();
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    } finally {
      if (mounted) setState(() => _refreshing = false);
    }
  }

  Future<void> _exportOpml() async {
    try {
      final opml = await AppScope.read(context).client.downloadOpml();
      await Clipboard.setData(ClipboardData(text: opml));
      if (!mounted) return;
      // No file picker dependency yet, so the clipboard is the honest export.
      showToast(context, 'OPML copied to clipboard.');
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  Future<void> _importOpml() async {
    final text = await showDialog<String>(
      context: context,
      builder: (_) => const _ImportOpmlDialog(),
    );
    if (text == null || text.isEmpty || !mounted) return;
    try {
      final added = await AppScope.read(context).client.uploadOpml(text);
      if (!mounted) return;
      showToast(context, 'Added $added feeds.');
      _load();
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  Future<void> _retry(Feed feed) async {
    try {
      await AppScope.read(context).client.retryFeed(feed.feedUrl);
      if (!mounted) return;
      showToast(context, 'Retry queued.');
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      floatingActionButton: FloatingActionButton(
        onPressed: _addFeed,
        shape: const RoundedRectangleBorder(),
        child: const Icon(Icons.add),
      ),
      body: Column(
        children: [
          _Toolbar(
            refreshing: _refreshing,
            onRefresh: _refreshAll,
            onImport: _importOpml,
            onExport: _exportOpml,
          ),
          Expanded(
            child: AsyncView<FeedsResponse>(
              future: _future,
              onRetry: _load,
              builder: (context, data) => _body(context, data),
            ),
          ),
        ],
      ),
    );
  }

  Widget _body(BuildContext context, FeedsResponse data) {
    if (data.subscriptions.isEmpty && data.deadFeeds.isEmpty) {
      return const EmptyView(message: 'No subscriptions yet.\nAdd a feed to get started.');
    }
    return RefreshIndicator(
      onRefresh: () async => _load(),
      child: ListView(
        children: [
          if (data.categories.isNotEmpty)
            _CategoryFilter(
              categories: data.categories,
              selected: _category,
              onChanged: (c) {
                _category = c;
                _page = 1;
                _load();
              },
            ),
          for (final s in data.subscriptions)
            _SubscriptionTile(
              subscription: s,
              onOpen: () => Navigator.of(context).push(MaterialPageRoute(
                builder: (_) => ArticlesScreen(feedUrl: s.feedUrl, title: s.feedTitle),
              )),
              onEdit: () => _editCategory(s),
              onRemove: () => _remove(s),
            ),
          if (data.deadFeeds.isNotEmpty) ...[
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 24, 16, 8),
              child: Text('Not responding', style: Theme.of(context).textTheme.titleMedium),
            ),
            for (final f in data.deadFeeds)
              _DeadFeedTile(feed: f, onRetry: () => _retry(f)),
          ],
          const SizedBox(height: 80),
        ],
      ),
    );
  }
}

class _Toolbar extends StatelessWidget {
  const _Toolbar({
    required this.refreshing,
    required this.onRefresh,
    required this.onImport,
    required this.onExport,
  });

  final bool refreshing;
  final VoidCallback onRefresh;
  final VoidCallback onImport;
  final VoidCallback onExport;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 12, 12, 4),
      child: Row(
        children: [
          GleanButton(label: 'Refresh', busy: refreshing, onPressed: onRefresh),
          const SizedBox(width: 8),
          GleanButton(label: 'Import', onPressed: onImport),
          const SizedBox(width: 8),
          GleanButton(label: 'Export', onPressed: onExport),
        ],
      ),
    );
  }
}

class _CategoryFilter extends StatelessWidget {
  const _CategoryFilter({
    required this.categories,
    required this.selected,
    required this.onChanged,
  });

  final List<String> categories;
  final String? selected;
  final ValueChanged<String?> onChanged;

  @override
  Widget build(BuildContext context) {
    final c = GleanColors.of(context);
    return SizedBox(
      height: 48,
      child: ListView(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        children: [
          for (final label in <String?>[null, ...categories])
            Padding(
              padding: const EdgeInsets.only(right: 8),
              child: GestureDetector(
                onTap: () => onChanged(label),
                child: Container(
                  alignment: Alignment.center,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  decoration: BoxDecoration(
                    color: selected == label ? c.accent : Colors.transparent,
                    border: Border.all(color: c.border, width: 2),
                  ),
                  child: Text(
                    label ?? 'All',
                    style: Theme.of(context).textTheme.labelLarge?.copyWith(
                          color: selected == label ? c.accentInk : c.fg,
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

class _SubscriptionTile extends StatelessWidget {
  const _SubscriptionTile({
    required this.subscription,
    required this.onOpen,
    required this.onEdit,
    required this.onRemove,
  });

  final Subscription subscription;
  final VoidCallback onOpen;
  final VoidCallback onEdit;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    final c = GleanColors.of(context);
    final text = Theme.of(context).textTheme;
    return InkWell(
      onTap: onOpen,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          border: Border(bottom: BorderSide(color: c.faint, width: 1)),
        ),
        child: Row(
          children: [
            FaviconBadge(url: subscription.faviconUrl, seed: subscription.feedTitle),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(subscription.feedTitle,
                      maxLines: 1, overflow: TextOverflow.ellipsis, style: text.bodyLarge),
                  if (subscription.category.isNotEmpty) ...[
                    const SizedBox(height: 4),
                    GleanTag(subscription.category),
                  ],
                ],
              ),
            ),
            if (subscription.unreadCount > 0) ...[
              const SizedBox(width: 8),
              GleanTag('${subscription.unreadCount}', emphasis: true),
            ],
            PopupMenuButton<String>(
              icon: Icon(Icons.more_vert, color: c.muted, size: 18),
              onSelected: (v) => v == 'edit' ? onEdit() : onRemove(),
              itemBuilder: (_) => const [
                PopupMenuItem(value: 'edit', child: Text('Change category')),
                PopupMenuItem(value: 'remove', child: Text('Unsubscribe')),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _DeadFeedTile extends StatelessWidget {
  const _DeadFeedTile({required this.feed, required this.onRetry});

  final Feed feed;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final c = GleanColors.of(context);
    final text = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
      child: GleanBox(
        filled: true,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(feed.title.isEmpty ? feed.feedUrl : feed.title, style: text.bodyMedium),
            const SizedBox(height: 4),
            Text(
              feed.lastError.isEmpty ? '${feed.errorCount} failures' : feed.lastError,
              style: text.bodySmall?.copyWith(color: c.danger),
            ),
            const SizedBox(height: 10),
            GleanButton(label: 'Retry', onPressed: onRetry),
          ],
        ),
      ),
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
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        TextButton(
          onPressed: () => Navigator.pop(context, _controller.text.trim()),
          child: const Text('Add'),
        ),
      ],
    );
  }
}

class _EditCategoryDialog extends StatefulWidget {
  const _EditCategoryDialog({required this.initial});

  final String initial;

  @override
  State<_EditCategoryDialog> createState() => _EditCategoryDialogState();
}

class _EditCategoryDialogState extends State<_EditCategoryDialog> {
  late final _controller = TextEditingController(text: widget.initial);

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      shape: const RoundedRectangleBorder(),
      title: const Text('Category'),
      content: TextField(
        controller: _controller,
        autofocus: true,
        decoration: const InputDecoration(hintText: 'news'),
        onSubmitted: (v) => Navigator.pop(context, v.trim()),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        TextButton(
          onPressed: () => Navigator.pop(context, _controller.text.trim()),
          child: const Text('Save'),
        ),
      ],
    );
  }
}

class _ImportOpmlDialog extends StatefulWidget {
  const _ImportOpmlDialog();

  @override
  State<_ImportOpmlDialog> createState() => _ImportOpmlDialogState();
}

class _ImportOpmlDialogState extends State<_ImportOpmlDialog> {
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
      title: const Text('Import OPML'),
      content: TextField(
        controller: _controller,
        autofocus: true,
        maxLines: 8,
        decoration: const InputDecoration(hintText: 'Paste OPML XML'),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        TextButton(
          onPressed: () => Navigator.pop(context, _controller.text),
          child: const Text('Import'),
        ),
      ],
    );
  }
}
