import 'dart:convert';

import 'package:http/http.dart' as http;

import '../models/models.dart';

/// Thrown for any non-2xx response, or one carrying `{"error": …}`.
class ApiException implements Exception {
  ApiException(this.message);
  final String message;
  @override
  String toString() => message;
}

/// The reader's API.
///
/// No cookies, no CSRF, no auth: the server is single-user and binds
/// loopback. That is why this class is a thin wrapper rather than a session.
class GleanClient {
  GleanClient({required this.baseUrl, http.Client? client})
      : _client = client ?? http.Client();

  final String baseUrl;
  final http.Client _client;

  Uri _uri(String path, [Map<String, String>? query]) {
    final q = query?.entries.where((e) => e.value.isNotEmpty);
    return Uri.parse('$baseUrl$path').replace(
      queryParameters: q == null || q.isEmpty ? null : Map.fromEntries(q),
    );
  }

  dynamic _decode(http.Response res) {
    late final dynamic body;
    try {
      body = jsonDecode(res.body.isEmpty ? 'null' : res.body);
    } on FormatException {
      throw ApiException('Unexpected response from server.');
    }
    if (body is Map && body['error'] != null) {
      throw ApiException('${body['error']}');
    }
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw ApiException('Request failed (${res.statusCode}).');
    }
    return body;
  }

  Future<dynamic> _get(String path, [Map<String, String>? q]) async =>
      _decode(await _client.get(_uri(path, q)));

  Future<dynamic> _post(String path, [Map<String, String>? form]) async =>
      _decode(await _client.post(_uri(path), body: form ?? const {}));

  // --- feeds ---

  Future<List<Feed>> feeds() async =>
      ((await _get('/feeds')) as List)
          .map((e) => Feed.fromJson(e as Map<String, dynamic>))
          .toList();

  Future<int> unreadCount() async =>
      ((await _get('/unread')) as Map<String, dynamic>)['count'] as int? ?? 0;

  /// Subscribe. The server fetches immediately and reports how many articles
  /// arrived, so a feed that resolves but is empty is distinguishable from
  /// one that failed.
  Future<int> addFeed(String url) async {
    final j = await _post('/feeds', {'url': url}) as Map<String, dynamic>;
    return (j['added'] as num?)?.toInt() ?? 0;
  }

  Future<void> removeFeed(String feedUrl) async =>
      _decode(await _client.delete(_uri('/feeds', {'url': feedUrl})));

  Future<RefreshResult> refresh() async =>
      RefreshResult.fromJson(await _post('/refresh') as Map<String, dynamic>);

  // --- articles ---

  Future<List<Article>> articles({
    String status = 'unread',
    String feedUrl = '',
    String search = '',
    int limit = 100,
  }) async {
    // A search spans everything; scoping it to a feed and a read state as
    // well would quietly return nothing for the common case.
    final q = search.isNotEmpty
        ? {'q': search, 'limit': '$limit'}
        : {'status': status, 'feed': feedUrl, 'limit': '$limit'};
    return ((await _get('/articles', q)) as List)
        .map((e) => Article.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<Article> article(int id) async =>
      Article.fromJson(await _get('/articles/$id') as Map<String, dynamic>);

  Future<void> setRead(int id, {bool read = true}) =>
      _post('/read', {'id': '$id', if (!read) 'undo': '1'});

  Future<int> markAllRead({String feedUrl = ''}) async {
    final j = await _post('/read-all', {if (feedUrl.isNotEmpty) 'feed': feedUrl})
        as Map<String, dynamic>;
    return (j['unread'] as num?)?.toInt() ?? 0;
  }

  /// Scrape the original page for feeds that only ship an excerpt.
  Future<String> fetchFullText(int id) async {
    final j = await _post('/fetch-content', {'id': '$id'})
        as Map<String, dynamic>;
    return (j['content'] as String?) ?? '';
  }

  void close() => _client.close();
}
