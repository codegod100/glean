import 'package:flutter/material.dart';

import '../api/responses.dart';
import '../app_state.dart';
import '../theme.dart';
import '../widgets/common.dart';
import 'article_screen.dart';

/// Public trending list. This is the one signed-out screen, so it doubles as
/// the landing page for visitors who have not signed in.
class TrendingScreen extends StatefulWidget {
  const TrendingScreen({super.key});

  @override
  State<TrendingScreen> createState() => _TrendingScreenState();
}

class _TrendingScreenState extends State<TrendingScreen> {
  Future<TrendingResponse>? _future;
  int _page = 1;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _load() {
    setState(() {
      _future = AppScope.read(context).client.trending(page: _page);
    });
  }

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    final c = GleanColors.of(context);

    return AsyncView<TrendingResponse>(
      future: _future,
      onRetry: _load,
      builder: (context, data) {
        if (data.trending.isEmpty) {
          return const EmptyView(message: 'Nothing trending yet.');
        }
        return RefreshIndicator(
          onRefresh: () async => _load(),
          child: ListView.builder(
            itemCount: data.trending.length + 1,
            itemBuilder: (context, i) {
              if (i == data.trending.length) {
                return Padding(
                  padding: const EdgeInsets.all(16),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: [
                      GleanButton(
                        label: 'Prev',
                        onPressed: data.pagination.hasPrev
                            ? () {
                                _page = data.pagination.prevPage;
                                _load();
                              }
                            : null,
                      ),
                      GleanButton(
                        label: 'Next',
                        onPressed: data.pagination.hasNext
                            ? () {
                                _page = data.pagination.nextPage;
                                _load();
                              }
                            : null,
                      ),
                    ],
                  ),
                );
              }
              final t = data.trending[i];
              return InkWell(
                onTap: () => Navigator.of(context).push(
                  MaterialPageRoute(builder: (_) => ArticleScreen(articleId: t.articleId)),
                ),
                child: Container(
                  padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
                  decoration: BoxDecoration(
                    border: Border(bottom: BorderSide(color: c.faint, width: 1)),
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Text('${i + 1 + (data.pagination.page - 1) * data.pagination.pageSize}',
                              style: text.bodySmall),
                          const SizedBox(width: 10),
                          FaviconBadge(url: t.faviconUrl, seed: t.feedTitle, size: 14),
                          const SizedBox(width: 6),
                          Expanded(
                            child: Text(t.feedTitle,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: text.bodySmall),
                          ),
                        ],
                      ),
                      const SizedBox(height: 6),
                      Text(t.title,
                          style: text.bodyLarge?.copyWith(fontWeight: FontWeight.w700)),
                      const SizedBox(height: 8),
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
      },
    );
  }
}
