import 'package:flutter/material.dart';

import '../api/responses.dart';
import '../app_state.dart';
import '../models/models.dart';
import '../widgets/article_tile.dart';
import '../widgets/common.dart';
import 'article_screen.dart';

/// Signed-in landing screen: unread count, latest articles, and the two
/// trending rails the server computes (personal and global).
class DashboardScreen extends StatefulWidget {
  const DashboardScreen({super.key});

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  Future<DashboardResponse>? _future;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _load() {
    setState(() {
      _future = AppScope.read(context).client.dashboard();
    });
  }

  @override
  Widget build(BuildContext context) {
    return AsyncView<DashboardResponse>(
      future: _future,
      onRetry: _load,
      builder: (context, data) => RefreshIndicator(
        onRefresh: () async => _load(),
        child: ListView(
          padding: const EdgeInsets.only(bottom: 32),
          children: [
            _Counts(data: data),
            if (data.articles.isNotEmpty) ...[
              const _SectionHeading('Latest'),
              for (final a in data.articles)
                ArticleTile(
                  article: a,
                  onTap: () => Navigator.of(context).push(
                    MaterialPageRoute(builder: (_) => ArticleScreen(articleId: a.id)),
                  ),
                ),
            ] else
              const Padding(
                padding: EdgeInsets.all(24),
                child: EmptyView(message: 'No unread articles.\nSubscribe to a feed to get started.'),
              ),
            if (data.personalTrending.isNotEmpty) ...[
              const _SectionHeading('Trending in your feeds'),
              _TrendingRail(items: data.personalTrending),
            ],
            if (data.globalTrending.isNotEmpty) ...[
              const _SectionHeading('Trending everywhere'),
              _TrendingRail(items: data.globalTrending),
            ],
          ],
        ),
      ),
    );
  }
}

class _Counts extends StatelessWidget {
  const _Counts({required this.data});

  final DashboardResponse data;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Row(
        children: [
          Expanded(child: _Stat(value: '${data.unreadCount}', label: 'unread')),
          const SizedBox(width: 12),
          Expanded(child: _Stat(value: '${data.subscriptionCount}', label: 'feeds')),
        ],
      ),
    );
  }
}

class _Stat extends StatelessWidget {
  const _Stat({required this.value, required this.label});

  final String value;
  final String label;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return GleanBox(
      filled: true,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(value, style: text.displaySmall),
          Text(label, style: text.bodySmall),
        ],
      ),
    );
  }
}

class _SectionHeading extends StatelessWidget {
  const _SectionHeading(this.label);

  final String label;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 10),
      child: Text(label, style: Theme.of(context).textTheme.titleMedium),
    );
  }
}

/// Horizontal rail of trending articles.
class _TrendingRail extends StatelessWidget {
  const _TrendingRail({required this.items});

  final List<TrendingItem> items;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return SizedBox(
      height: 168,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        itemCount: items.length,
        separatorBuilder: (_, _) => const SizedBox(width: 12),
        itemBuilder: (context, i) {
          final t = items[i];
          return SizedBox(
            width: 240,
            child: GleanBox(
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute(builder: (_) => ArticleScreen(articleId: t.articleId)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      FaviconBadge(url: t.faviconUrl, seed: t.feedTitle, size: 14),
                      const SizedBox(width: 6),
                      Expanded(
                        child: Text(t.feedTitle,
                            maxLines: 1, overflow: TextOverflow.ellipsis, style: text.bodySmall),
                      ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  Expanded(
                    child: Text(
                      t.title,
                      maxLines: 4,
                      overflow: TextOverflow.ellipsis,
                      style: text.bodyMedium?.copyWith(fontWeight: FontWeight.w700),
                    ),
                  ),
                  const SizedBox(height: 6),
                  Row(
                    children: [
                      GleanTag('${t.likeCount} likes'),
                      const SizedBox(width: 6),
                      if (t.annotationCount > 0) GleanTag('${t.annotationCount} notes'),
                    ],
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}
