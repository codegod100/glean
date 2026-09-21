import 'package:flutter/material.dart';

import '../api/client.dart';
import '../api/responses.dart';
import '../app_state.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/article_tile.dart';
import '../widgets/common.dart';
import 'article_screen.dart';

/// Saved things: liked articles and the reader's own annotations, each
/// paginated independently by the server (liked_page / annot_page).
class LibraryScreen extends StatefulWidget {
  const LibraryScreen({super.key});

  @override
  State<LibraryScreen> createState() => _LibraryScreenState();
}

class _LibraryScreenState extends State<LibraryScreen> {
  Future<LibraryResponse>? _future;
  int _likedPage = 1;
  int _annotPage = 1;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _load() {
    setState(() {
      _future = AppScope.read(context)
          .client
          .library(likedPage: _likedPage, annotPage: _annotPage);
    });
  }

  Future<void> _deleteAnnotation(int id) async {
    try {
      await AppScope.read(context).client.deleteAnnotation(id);
      if (!mounted) return;
      _load();
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 2,
      child: Column(
        children: [
          const TabBar(tabs: [Tab(text: 'Liked'), Tab(text: 'Notes')]),
          Expanded(
            child: AsyncView<LibraryResponse>(
              future: _future,
              onRetry: _load,
              builder: (context, data) => TabBarView(
                children: [
                  _liked(context, data),
                  _notes(context, data),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _liked(BuildContext context, LibraryResponse data) {
    if (data.articles.isEmpty) {
      return const EmptyView(message: 'Nothing liked yet.');
    }
    return ListView.builder(
      itemCount: data.articles.length + 1,
      itemBuilder: (context, i) {
        if (i == data.articles.length) {
          return _Pager(
            pagination: data.likedPage,
            onPage: (p) {
              _likedPage = p;
              _load();
            },
          );
        }
        final a = data.articles[i];
        return ArticleTile(
          article: a,
          onTap: () => Navigator.of(context).push(
            MaterialPageRoute(builder: (_) => ArticleScreen(articleId: a.id)),
          ),
        );
      },
    );
  }

  Widget _notes(BuildContext context, LibraryResponse data) {
    if (data.annotations.isEmpty) {
      return const EmptyView(message: 'No annotations yet.');
    }
    final text = Theme.of(context).textTheme;
    return ListView.builder(
      padding: const EdgeInsets.all(16),
      itemCount: data.annotations.length + 1,
      itemBuilder: (context, i) {
        if (i == data.annotations.length) {
          return _Pager(
            pagination: data.annotPage,
            onPage: (p) {
              _annotPage = p;
              _load();
            },
          );
        }
        final an = data.annotations[i];
        return Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: GleanBox(
            filled: true,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(child: Text(relativeTime(an.createdAt), style: text.bodySmall)),
                    InkWell(
                      onTap: () => _deleteAnnotation(an.id),
                      child: Icon(Icons.delete_outline,
                          size: 18, color: GleanColors.of(context).muted),
                    ),
                  ],
                ),
                if (an.quote.isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Text('"${an.quote}"',
                      style: text.bodyMedium?.copyWith(fontStyle: FontStyle.italic)),
                ],
                if (an.note.isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Text(an.note, style: text.bodyMedium),
                ],
                if (an.tags.isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: [for (final t in an.tags) GleanTag(t)],
                  ),
                ],
                if (an.articleId != null) ...[
                  const SizedBox(height: 10),
                  GleanButton(
                    label: 'Open article',
                    onPressed: () => Navigator.of(context).push(
                      MaterialPageRoute(
                        builder: (_) => ArticleScreen(articleId: an.articleId!),
                      ),
                    ),
                  ),
                ],
              ],
            ),
          ),
        );
      },
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
          GleanButton(
            label: 'Next',
            onPressed: pagination.hasNext ? () => onPage(pagination.nextPage) : null,
          ),
        ],
      ),
    );
  }
}
