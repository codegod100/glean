import 'dart:convert';

import 'package:http/http.dart' as http;

import '../models/models.dart';
import 'responses.dart';
import 'session.dart';

/// Thrown for any non-2xx API response. The server puts a human-readable
/// message in `{"error": "..."}` (writeAPIError), so surface that when present.
class ApiException implements Exception {
  ApiException(this.statusCode, this.message);

  final int statusCode;
  final String message;

  bool get isUnauthorized => statusCode == 401 || statusCode == 403;

  @override
  String toString() => message;
}

/// Typed wrapper over the Glean JSON API (internal/server/server.go).
///
/// Every method here corresponds to exactly one route; the grouping and order
/// follow setupRoutes() so the two can be diffed by eye.
class GleanClient {
  GleanClient(this.session);

  final GleanSession session;

  // --- plumbing ---

  Map<String, dynamic> _decode(http.Response res) {
    final body = res.body.isEmpty ? '{}' : res.body;
    late final Map<String, dynamic> json;
    try {
      json = jsonDecode(body) as Map<String, dynamic>;
    } on FormatException {
      // A non-JSON body means we hit something other than the API -- a proxy
      // error page, or an HTML redirect from an expired session.
      throw ApiException(res.statusCode, 'Unexpected non-JSON response from server.');
    }
    if (res.statusCode >= 200 && res.statusCode < 300) return json;
    final msg = (json['error'] as String?) ?? 'Request failed (${res.statusCode}).';
    throw ApiException(res.statusCode, msg);
  }

  Future<Map<String, dynamic>> _get(String path, [Map<String, dynamic>? q]) async =>
      _decode(await session.get(path, q));

  Future<Map<String, dynamic>> _post(String path,
          {Map<String, dynamic>? query, Map<String, String>? form}) async =>
      _decode(await session.send('POST', path, query: query, form: form));

  Future<Map<String, dynamic>> _delete(String path,
          {Map<String, dynamic>? query, Map<String, String>? form}) async =>
      _decode(await session.send('DELETE', path, query: query, form: form));

  // --- identity ---

  Future<MeResponse> me() async => MeResponse.fromJson(await _get('/api/me'));

  // --- dashboard ---

  Future<DashboardResponse> dashboard() async =>
      DashboardResponse.fromJson(await _get('/api/dashboard/'));

  // --- articles ---

  Future<ArticlesResponse> articles({
    int page = 1,
    String? feedUrl,
    String? status,
    String? search,
    String? category,
    bool sortOldest = false,
  }) async =>
      ArticlesResponse.fromJson(await _get('/api/articles/', {
        'page': page,
        'feed': feedUrl,
        'status': status,
        'q': search,
        'category': category,
        if (sortOldest) 'sort': 'oldest',
      }));

  Future<ArticleDetailResponse> article(int id) async =>
      ArticleDetailResponse.fromJson(await _get('/api/articles/$id'));

  Future<int> newArticleCount() async =>
      jsonInt((await _get('/api/articles/new-count'))['count']);

  Future<bool> markRead(int id) async =>
      jsonBool((await _post('/api/articles/$id/read'))['is_read']);

  Future<bool> markUnread(int id) async =>
      jsonBool((await _post('/api/articles/$id/unread'))['is_read']);

  Future<LikeResult> likeArticle(int id) async =>
      LikeResult.fromJson(await _post('/api/articles/$id/like'));

  Future<String> fetchContent(int id) async =>
      jsonStr((await _post('/api/articles/$id/fetch-content'))['full_content']);

  Future<void> markAllRead({String? feedUrl, String? category}) =>
      _post('/api/articles/mark-all-read', form: {
        'feed': ?feedUrl,
        'category': ?category,
      });

  // --- feeds ---

  Future<FeedsResponse> feeds({int page = 1, String? category}) async =>
      FeedsResponse.fromJson(await _get('/api/feeds/', {'page': page, 'category': category}));

  Future<Subscription> addFeed(String url, {String? category}) async {
    final j = await _post('/api/feeds/add', form: {
      'feed_url': url,
      'category': ?category,
    });
    return Subscription.fromJson(j['subscription'] as Map<String, dynamic>);
  }

  Future<Subscription> editFeed(String feedUrl, {String? category}) async {
    final j = await _post('/api/feeds/edit', form: {
      'feed_url': feedUrl,
      'category': ?category,
    });
    return Subscription.fromJson(j['subscription'] as Map<String, dynamic>);
  }

  Future<void> removeFeed(String feedUrl) =>
      _delete('/api/feeds/remove', form: {'url': feedUrl});

  Future<int> uploadOpml(String opml) async {
    final res = await session.sendFile(
      '/api/feeds/opml/upload',
      field: 'opml',
      filename: 'subscriptions.opml',
      bytes: utf8.encode(opml),
    );
    return jsonInt(_decode(res)['added']);
  }

  Future<String> downloadOpml() async {
    final res = await session.get('/api/feeds/opml/download');
    if (res.statusCode >= 300) {
      throw ApiException(res.statusCode, 'Could not export subscriptions.');
    }
    return res.body;
  }

  Future<void> refreshFeeds() => _post('/api/feeds/refresh');

  Future<void> retryFeed(String feedUrl) =>
      _post('/api/feeds/retry', form: {'url': feedUrl});

  Future<List<Subscription>> feedList() async =>
      jsonList((await _get('/api/feeds/list'))['subscriptions'], Subscription.fromJson);

  Future<void> clearAllSubscriptions() => _post('/api/feeds/clear');

  // --- trending ---

  Future<TrendingResponse> trending({int page = 1, String? scope}) async =>
      TrendingResponse.fromJson(await _get('/api/trending/', {'page': page, 'scope': scope}));

  // --- profile ---

  Future<ProfileResponse> profile(String did) async =>
      ProfileResponse.fromJson(await _get('/api/profile/$did'));

  // --- library ---

  Future<LibraryResponse> library({int likedPage = 1, int annotPage = 1}) async =>
      LibraryResponse.fromJson(
          await _get('/api/library/', {'liked_page': likedPage, 'annot_page': annotPage}));

  Future<Annotation> createAnnotation({
    required String articleUrl,
    required String feedUrl,
    String quote = '',
    String note = '',
    List<String> tags = const [],
    int? rating,
  }) async {
    final j = await _post('/api/library/create', form: {
      'article_url': articleUrl,
      'feed_url': feedUrl,
      'quote': quote,
      'note': note,
      'tags': tags.join(','),
      if (rating != null) 'rating': '$rating',
    });
    return Annotation.fromJson(j['annotation'] as Map<String, dynamic>);
  }

  Future<void> deleteAnnotation(int id) => _post('/api/library/$id/delete');

  // --- recommendations ---

  Future<List<Article>> articleRecs() async =>
      jsonList((await _get('/api/recs/articles'))['articles'], Article.fromJson);

  Future<FeedRecsResponse> feedRecs() async =>
      FeedRecsResponse.fromJson(await _get('/api/recs/feeds'));

  Future<PeopleRecsResponse> peopleRecs() async =>
      PeopleRecsResponse.fromJson(await _get('/api/recs/people'));

  Future<void> dismissFeedRec(String feedUrl) =>
      _post('/api/recs/dismiss-feed', form: {'feed_url': feedUrl});

  Future<void> dismissArticleRec(String articleUrl) =>
      _post('/api/recs/dismiss-article', form: {'article_url': articleUrl});

  Future<void> dismissPersonRec(String did) =>
      _post('/api/recs/dismiss-person', form: {'target_did': did});

  // --- settings ---

  Future<List<String>> toggleLanguage(String code) async =>
      jsonStrList((await _post('/api/settings/languages/$code'))['languages']);

  Future<bool> setExpandedView(bool enabled) async => jsonBool(
        (await _post('/api/settings/expanded-view',
            form: {'expanded_view': enabled ? '1' : '0'}))['expanded_view'],
      );

  Future<bool> setDigestEnabled(bool enabled) async => jsonBool(
        (await _post('/api/settings/digest-enabled',
            form: {'digest_enabled': enabled ? '1' : '0'}))['digest_enabled'],
      );

  // --- digest ---

  Future<Map<String, dynamic>> digest() => _get('/api/digest');

  Future<void> markDigestRead() => _post('/api/digest/mark-read');

  // --- auth ---

  /// Resolve a partial handle to candidate accounts for the login field.
  Future<List<Map<String, dynamic>>> resolveActors(String query) async {
    final j = await _get('/api/auth/actors', {'q': query});
    return ((j['actors'] as List?) ?? const []).cast<Map<String, dynamic>>();
  }

  /// Returns the URL the user must visit to authorize. The server answers with
  /// `{"redirect": ...}` -- either the PDS authorization endpoint, or
  /// `/dashboard` when it fell back to creating a read-only account.
  Future<String> startAuth(String handle) async =>
      jsonStr((await _post('/api/auth/start', form: {'handle': handle}))['redirect']);

  Future<void> logout() => _post('/api/auth/logout');
}
