import 'package:flutter/material.dart';

import '../api/client.dart';
import '../api/responses.dart';
import '../app_state.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/article_tile.dart';
import '../widgets/common.dart';
import 'article_screen.dart';
import 'profile_screen.dart';

/// The three recommendation rails the clustering engine produces: articles,
/// feeds and people. Each is independently fetched so one slow or empty rail
/// does not hold up the others.
class DiscoverScreen extends StatefulWidget {
  const DiscoverScreen({super.key});

  @override
  State<DiscoverScreen> createState() => _DiscoverScreenState();
}

class _DiscoverScreenState extends State<DiscoverScreen> {
  Future<List<Article>>? _articles;
  Future<FeedRecsResponse>? _feeds;
  Future<PeopleRecsResponse>? _people;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _load() {
    final client = AppScope.read(context).client;
    setState(() {
      _articles = client.articleRecs();
      _feeds = client.feedRecs();
      _people = client.peopleRecs();
    });
  }

  Future<void> _run(Future<void> Function() action, String done) async {
    try {
      await action();
      if (!mounted) return;
      showToast(context, done);
      _load();
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 3,
      child: Column(
        children: [
          const TabBar(tabs: [
            Tab(text: 'Articles'),
            Tab(text: 'Feeds'),
            Tab(text: 'People'),
          ]),
          Expanded(
            child: TabBarView(
              children: [
                AsyncView<List<Article>>(
                  future: _articles,
                  onRetry: _load,
                  builder: (context, items) => items.isEmpty
                      ? const EmptyView(message: 'No article suggestions yet.')
                      : ListView(children: [
                          for (final a in items)
                            Dismissible(
                              key: ValueKey('rec-article-${a.id}'),
                              direction: DismissDirection.endToStart,
                              background: const _DismissBackground(),
                              onDismissed: (_) => _run(
                                () => AppScope.read(context).client.dismissArticleRec(a.url),
                                'Dismissed.',
                              ),
                              child: ArticleTile(
                                article: a,
                                onTap: () => Navigator.of(context).push(
                                  MaterialPageRoute(
                                    builder: (_) => ArticleScreen(articleId: a.id),
                                  ),
                                ),
                              ),
                            ),
                        ]),
                ),
                AsyncView<FeedRecsResponse>(
                  future: _feeds,
                  onRetry: _load,
                  builder: (context, data) => data.feeds.isEmpty
                      ? const EmptyView(message: 'No feed suggestions yet.')
                      : ListView(
                          padding: const EdgeInsets.all(16),
                          children: [
                            for (final f in data.feeds)
                              _FeedRecCard(
                                rec: f,
                                onSubscribe: () => _run(
                                  () => AppScope.read(context).client.addFeed(f.feedUrl),
                                  'Subscribed.',
                                ),
                                onDismiss: () => _run(
                                  () => AppScope.read(context).client.dismissFeedRec(f.feedUrl),
                                  'Dismissed.',
                                ),
                              ),
                          ],
                        ),
                ),
                AsyncView<PeopleRecsResponse>(
                  future: _people,
                  onRetry: _load,
                  builder: (context, data) {
                    final all = [...data.followed, ...data.discover];
                    if (all.isEmpty) {
                      return const EmptyView(message: 'No people suggestions yet.');
                    }
                    return ListView(
                      padding: const EdgeInsets.all(16),
                      children: [
                        for (final p in all)
                          _PersonCard(
                            person: p,
                            onOpen: () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => ProfileScreen(did: p.did)),
                            ),
                            onDismiss: () => _run(
                              () => AppScope.read(context).client.dismissPersonRec(p.did),
                              'Dismissed.',
                            ),
                          ),
                      ],
                    );
                  },
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _DismissBackground extends StatelessWidget {
  const _DismissBackground();

  @override
  Widget build(BuildContext context) {
    final c = GleanColors.of(context);
    return Container(
      color: c.danger,
      alignment: Alignment.centerRight,
      padding: const EdgeInsets.only(right: 20),
      child: Text('Dismiss',
          style: Theme.of(context).textTheme.labelLarge?.copyWith(color: c.bg)),
    );
  }
}

class _FeedRecCard extends StatelessWidget {
  const _FeedRecCard({
    required this.rec,
    required this.onSubscribe,
    required this.onDismiss,
  });

  final FeedRecommendation rec;
  final VoidCallback onSubscribe;
  final VoidCallback onDismiss;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: GleanBox(
        filled: true,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                FaviconBadge(url: rec.faviconUrl, seed: rec.title),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(rec.title.isEmpty ? rec.feedUrl : rec.title,
                      maxLines: 1, overflow: TextOverflow.ellipsis, style: text.bodyLarge),
                ),
              ],
            ),
            if (rec.description.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(rec.description,
                  maxLines: 3, overflow: TextOverflow.ellipsis, style: text.bodySmall),
            ],
            const SizedBox(height: 10),
            Row(
              children: [
                GleanTag('${rec.subscriberCount} readers'),
                const Spacer(),
                GleanButton(label: 'Dismiss', onPressed: onDismiss),
                const SizedBox(width: 8),
                GleanButton(label: 'Subscribe', accent: true, onPressed: onSubscribe),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _PersonCard extends StatelessWidget {
  const _PersonCard({
    required this.person,
    required this.onOpen,
    required this.onDismiss,
  });

  final PersonRecommendation person;
  final VoidCallback onOpen;
  final VoidCallback onDismiss;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    // The engine explains its suggestion by overlap; showing that is more
    // useful than the raw Jaccard score.
    final reasons = <String>[
      if (person.commonFeeds > 0) '${person.commonFeeds} shared feeds',
      if (person.commonLikes > 0) '${person.commonLikes} shared likes',
      if (person.commonTags > 0) '${person.commonTags} shared tags',
    ];
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: GleanBox(
        filled: true,
        onTap: onOpen,
        child: Row(
          children: [
            FaviconBadge(url: person.avatarUrl, seed: person.label, size: 36),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(person.label,
                      maxLines: 1, overflow: TextOverflow.ellipsis, style: text.bodyLarge),
                  Text('@${person.handle}',
                      maxLines: 1, overflow: TextOverflow.ellipsis, style: text.bodySmall),
                  if (reasons.isNotEmpty) ...[
                    const SizedBox(height: 6),
                    Text(reasons.join(' · '), style: text.bodySmall),
                  ],
                ],
              ),
            ),
            if (person.isFollowed) const GleanTag('following'),
            IconButton(
              icon: const Icon(Icons.close, size: 18),
              onPressed: onDismiss,
              tooltip: 'Dismiss',
            ),
          ],
        ),
      ),
    );
  }
}
