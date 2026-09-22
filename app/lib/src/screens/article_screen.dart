import 'package:flutter/material.dart';
import 'package:flutter_widget_from_html_core/flutter_widget_from_html_core.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/client.dart';
import '../app_state.dart';
import '../models/models.dart';
import '../theme.dart';
import '../widgets/common.dart';

/// Reader view for one article.
class ArticleScreen extends StatefulWidget {
  const ArticleScreen({super.key, required this.articleId});

  final int articleId;

  @override
  State<ArticleScreen> createState() => _ArticleScreenState();
}

class _ArticleScreenState extends State<ArticleScreen> {
  Future<Article>? _future;
  Article? _article;
  bool _fetching = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _load() {
    setState(() {
      _future = _fetch();
    });
  }

  Future<Article> _fetch() async {
    final client = AppScope.read(context).client;
    final a = await client.article(widget.articleId);
    _article = a;
    // Opening is what marks it read, as in the web interface. Best-effort:
    // failing to record it must not break the reader.
    if (!a.isRead) {
      client.setRead(a.id).catchError((_) {});
    }
    return a;
  }

  Future<void> _fetchFullText() async {
    setState(() => _fetching = true);
    try {
      final text = await AppScope.read(context).client
          .fetchFullText(widget.articleId);
      if (!mounted) return;
      setState(() => _article = _article?.copyWith(content: text));
    } on ApiException catch (e) {
      if (mounted) showToast(context, e.message);
    } finally {
      if (mounted) setState(() => _fetching = false);
    }
  }

  Future<void> _openOriginal() async {
    final url = _article?.url ?? '';
    final uri = Uri.tryParse(url);
    if (uri == null || url.isEmpty) return;
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
          IconButton(
            tooltip: 'Open original',
            icon: const Icon(Icons.open_in_new),
            onPressed: _openOriginal,
          ),
        ],
      ),
      body: AsyncView<Article>(
        future: _future,
        onRetry: _load,
        builder: (context, data) => _body(context, _article ?? data),
      ),
    );
  }

  Widget _body(BuildContext context, Article a) {
    final text = Theme.of(context).textTheme;
    final c = GleanColors.of(context);

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            Expanded(child: Text(a.feedTitle, style: text.bodySmall)),
            Text(relativeTime(a.published), style: text.bodySmall),
          ],
        ),
        const SizedBox(height: 12),
        Text(a.title.isEmpty ? '(untitled)' : a.title, style: text.headlineSmall),
        if (a.author.isNotEmpty) ...[
          const SizedBox(height: 6),
          Text(a.author, style: text.bodySmall),
        ],
        const SizedBox(height: 16),
        Divider(color: c.faint, height: 1),
        const SizedBox(height: 16),
        if (a.content.isEmpty)
          Text('No content in the feed.', style: text.bodySmall)
        else
          // Rendering HTML from someone else's server is only safe because
          // the reader sanitises every body it sends through the same
          // whitelist its scraper uses.
          HtmlWidget(
            a.content,
            textStyle: text.bodyLarge,
            onTapUrl: (url) async {
              final uri = Uri.tryParse(url);
              if (uri != null) {
                await launchUrl(uri, mode: LaunchMode.externalApplication);
              }
              return true;
            },
          ),
        const SizedBox(height: 24),
        GleanButton(
          label: 'Fetch full text',
          busy: _fetching,
          onPressed: _fetchFullText,
        ),
        const SizedBox(height: 40),
      ],
    );
  }
}
