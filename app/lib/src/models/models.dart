/// The two shapes the reader's API returns.
///
/// Deliberately small. The server has no accounts, no pagination envelopes
/// and no social records, so there is nothing here but articles and the
/// feeds they came from.
library;

String _str(Object? v) => (v as String?) ?? '';
int _int(Object? v) => (v as num?)?.toInt() ?? 0;
bool _bool(Object? v) => (v as bool?) ?? false;

class Article {
  const Article({
    required this.id,
    required this.feedUrl,
    required this.feedTitle,
    required this.title,
    required this.url,
    required this.author,
    required this.summary,
    required this.content,
    required this.published,
    required this.isRead,
  });

  final int id;
  final String feedUrl;
  final String feedTitle;
  final String title;
  final String url;
  final String author;
  final String summary;

  /// Sanitised HTML. The server runs every body through a whitelist before
  /// sending it, whether it was scraped or came straight from the feed.
  final String content;
  final DateTime? published;
  final bool isRead;

  factory Article.fromJson(Map<String, dynamic> j) => Article(
    id: _int(j['id']),
    feedUrl: _str(j['feed_url']),
    feedTitle: _str(j['feed_title']),
    title: _str(j['title']),
    url: _str(j['url']),
    author: _str(j['author']),
    summary: _str(j['summary']),
    content: _str(j['content']),
    published: DateTime.tryParse(_str(j['published']))?.toLocal(),
    isRead: _bool(j['is_read']),
  );

  Article copyWith({bool? isRead, String? content}) => Article(
    id: id,
    feedUrl: feedUrl,
    feedTitle: feedTitle,
    title: title,
    url: url,
    author: author,
    summary: summary,
    content: content ?? this.content,
    published: published,
    isRead: isRead ?? this.isRead,
  );
}

class Feed {
  const Feed({
    required this.feedUrl,
    required this.title,
    required this.category,
    required this.unread,
    required this.faviconUrl,
  });

  final String feedUrl;
  final String title;
  final String category;
  final int unread;
  final String faviconUrl;

  /// The "all feeds" row, which the server does not send.
  factory Feed.all(int unread) => Feed(
    feedUrl: '',
    title: 'All feeds',
    category: '',
    unread: unread,
    faviconUrl: '',
  );

  bool get isAll => feedUrl.isEmpty;

  factory Feed.fromJson(Map<String, dynamic> j) => Feed(
    feedUrl: _str(j['feed_url']),
    title: _str(j['title']),
    category: _str(j['category']),
    unread: _int(j['unread']),
    faviconUrl: _str(j['favicon_url']),
  );
}

/// What a refresh reports back: how many arrived, and which feeds failed.
class RefreshResult {
  const RefreshResult({required this.added, required this.errors});

  final int added;
  final List<String> errors;

  factory RefreshResult.fromJson(Map<String, dynamic> j) => RefreshResult(
    added: _int(j['added']),
    errors: ((j['errors'] as List?) ?? const [])
        .map((e) => _str((e as Map<String, dynamic>)['feed_url']))
        .toList(),
  );
}

class ImportResult {
  const ImportResult({
    required this.imported,
    required this.added,
    required this.errors,
  });
  final int imported;
  final int added;
  final List<Map<String, dynamic>> errors;

  factory ImportResult.fromJson(Map<String, dynamic> json) => ImportResult(
    imported: (json['imported'] as num?)?.toInt() ?? 0,
    added: (json['added'] as num?)?.toInt() ?? 0,
    errors: ((json['errors'] as List?) ?? const [])
        .whereType<Map>()
        .map((e) => Map<String, dynamic>.from(e))
        .toList(),
  );
}
