import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_widget_from_html_core/flutter_widget_from_html_core.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/client.dart';
import '../api/responses.dart';
import '../app_state.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/common.dart';

/// Reader view for one article, plus its annotations.
///
/// Feeds usually carry truncated HTML, so this offers the server's scraper
/// (/api/articles/{id}/fetch-content) rather than sending the reader to the
/// browser for the full text.
class ArticleScreen extends StatefulWidget {
  const ArticleScreen({super.key, required this.articleId});

  final int articleId;

  @override
  State<ArticleScreen> createState() => _ArticleScreenState();
}

class _ArticleScreenState extends State<ArticleScreen> {
  Future<ArticleDetailResponse>? _future;
  Article? _article;
  bool _fetching = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _load() {
    setState(() {
      _article = null;
      _future = _fetch();
    });
  }

  Future<ArticleDetailResponse> _fetch() async {
    final client = AppScope.read(context).client;
    final detail = await client.article(widget.articleId);
    _article = detail.article;
    // Opening an article is what marks it read in the web app too. It is
    // best-effort: failing to record it must not break the reader.
    if (!detail.article.isRead) {
      unawaited(client.markRead(widget.articleId).catchError((_) => false));
    }
    return detail;
  }

  Future<void> _toggleLike() async {
    final a = _article;
    if (a == null) return;
    final client = AppScope.read(context).client;
    setState(() => _article = a.copyWith(
          hasLiked: !a.hasLiked,
          likeCount: a.likeCount + (a.hasLiked ? -1 : 1),
        ));
    try {
      final r = await client.likeArticle(a.id);
      if (mounted) {
        setState(() => _article = a.copyWith(hasLiked: r.liked, likeCount: r.likeCount));
      }
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _article = a);
      showToast(context, e.message);
    }
  }

  Future<void> _fetchFullContent() async {
    setState(() => _fetching = true);
    try {
      final full = await AppScope.read(context).client.fetchContent(widget.articleId);
      if (!mounted) return;
      setState(() => _article = _article?.copyWith(fullContent: full));
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    } finally {
      if (mounted) setState(() => _fetching = false);
    }
  }

  Future<void> _openOriginal(String url) async {
    if (url.isEmpty) return;
    final uri = Uri.tryParse(url);
    if (uri == null) return;
    if (!await launchUrl(uri, mode: LaunchMode.externalApplication)) {
      if (mounted) showToast(context, 'Could not open the link.');
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Read'),
        actions: [
          if (_article != null)
            IconButton(
              tooltip: _article!.hasLiked ? 'Unlike' : 'Like',
              icon: Icon(
                _article!.hasLiked ? Icons.favorite : Icons.favorite_border,
                color: _article!.hasLiked ? GleanColors.of(context).danger : null,
              ),
              onPressed: _toggleLike,
            ),
          IconButton(
            tooltip: 'Open original',
            icon: const Icon(Icons.open_in_new),
            onPressed: () => _openOriginal(_article?.url ?? ''),
          ),
        ],
      ),
      body: AsyncView<ArticleDetailResponse>(
        future: _future,
        onRetry: _load,
        builder: (context, data) => _body(context, data),
      ),
    );
  }

  Widget _body(BuildContext context, ArticleDetailResponse data) {
    final a = _article ?? data.article;
    final text = Theme.of(context).textTheme;
    final c = GleanColors.of(context);

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            FaviconBadge(url: a.feedFaviconUrl, seed: a.feedTitle),
            const SizedBox(width: 8),
            Expanded(child: Text(a.feedTitle, style: text.bodySmall)),
            Text(relativeTime(a.published), style: text.bodySmall),
          ],
        ),
        const SizedBox(height: 12),
        Text(a.title, style: text.headlineSmall),
        if (a.author.isNotEmpty) ...[
          const SizedBox(height: 6),
          Text(a.author, style: text.bodySmall),
        ],
        const SizedBox(height: 16),
        Divider(color: c.faint, height: 1),
        const SizedBox(height: 16),
        if (a.readable.isEmpty)
          Text('No content in the feed.', style: text.bodySmall)
        else
          HtmlWidget(
            a.readable,
            textStyle: text.bodyLarge,
            onTapUrl: (url) async {
              await _openOriginal(url);
              return true;
            },
            customStylesBuilder: (element) => switch (element.localName) {
              'a' => {'color': _hex(c.accent), 'text-decoration': 'underline'},
              'blockquote' => {'border-left': '3px solid ${_hex(c.faint)}', 'padding-left': '12px'},
              'pre' || 'code' => {'font-size': '13px'},
              _ => null,
            },
          ),
        const SizedBox(height: 20),
        if (a.fullContent.isEmpty)
          GleanButton(
            label: 'Fetch full article',
            busy: _fetching,
            onPressed: _fetchFullContent,
          ),
        if (data.annotations.isNotEmpty) ...[
          const SizedBox(height: 28),
          Text('Annotations', style: text.titleMedium),
          const SizedBox(height: 12),
          for (final an in data.annotations) _AnnotationCard(annotation: an),
        ],
        const SizedBox(height: 40),
      ],
    );
  }
}

class _AnnotationCard extends StatelessWidget {
  const _AnnotationCard({required this.annotation});

  final Annotation annotation;

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
            Text('@${annotation.authorHandle}', style: text.bodySmall),
            if (annotation.quote.isNotEmpty) ...[
              const SizedBox(height: 8),
              Text('"${annotation.quote}"',
                  style: text.bodyMedium?.copyWith(fontStyle: FontStyle.italic)),
            ],
            if (annotation.note.isNotEmpty) ...[
              const SizedBox(height: 8),
              Text(annotation.note, style: text.bodyMedium),
            ],
            if (annotation.tags.isNotEmpty) ...[
              const SizedBox(height: 8),
              Wrap(
                spacing: 6,
                runSpacing: 6,
                children: [for (final t in annotation.tags) GleanTag(t)],
              ),
            ],
          ],
        ),
      ),
    );
  }
}

/// flutter_widget_from_html takes CSS strings, so theme colors have to be
/// handed over as hex rather than as Color objects.
String _hex(Color c) =>
    '#${((c.a * 255).round() << 24 | (c.r * 255).round() << 16 | (c.g * 255).round() << 8 | (c.b * 255).round()).toRadixString(16).padLeft(8, '0').substring(2)}';
