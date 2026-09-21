/// Dart mirrors of the wire types in internal/server/api.go.
///
/// Field names and nullability follow the Go structs exactly: a Go `*time.Time`
/// is a nullable DateTime here, and a `string` that the server coerces from
/// sql.NullString is a non-null String defaulting to "". Keeping the two in
/// step matters more than Dart-idiomatic naming, so the JSON keys are the
/// source of truth.
library;

DateTime? _time(Object? v) =>
    v == null ? null : DateTime.tryParse(v as String)?.toLocal();

String _str(Object? v) => (v as String?) ?? '';
int _int(Object? v) => (v as num?)?.toInt() ?? 0;
double _dbl(Object? v) => (v as num?)?.toDouble() ?? 0;
bool _bool(Object? v) => (v as bool?) ?? false;

List<String> _strList(Object? v) =>
    (v as List?)?.map((e) => e as String).toList() ?? const [];

List<T> _list<T>(Object? v, T Function(Map<String, dynamic>) f) =>
    (v as List?)?.map((e) => f(e as Map<String, dynamic>)).toList() ?? <T>[];

class User {
  const User({
    required this.did,
    required this.handle,
    required this.displayName,
    required this.avatarUrl,
  });

  final String did;
  final String handle;
  final String displayName;
  final String avatarUrl;

  /// The handle is the stable identity; display name is optional decoration.
  String get label => displayName.isNotEmpty ? displayName : handle;

  static const empty = User(did: '', handle: '', displayName: '', avatarUrl: '');

  factory User.fromJson(Map<String, dynamic> j) => User(
        did: _str(j['did']),
        handle: _str(j['handle']),
        displayName: _str(j['display_name']),
        avatarUrl: _str(j['avatar_url']),
      );
}

class Article {
  const Article({
    required this.id,
    required this.feedUrl,
    required this.feedTitle,
    required this.feedFaviconUrl,
    required this.title,
    required this.url,
    required this.author,
    required this.summary,
    required this.content,
    required this.fullContent,
    required this.published,
    required this.updated,
    required this.isRead,
    required this.likeCount,
    required this.hasLiked,
  });

  final int id;
  final String feedUrl;
  final String feedTitle;
  final String feedFaviconUrl;
  final String title;
  final String url;
  final String author;
  final String summary;
  final String content;
  final String fullContent;
  final DateTime? published;
  final DateTime? updated;
  final bool isRead;
  final int likeCount;
  final bool hasLiked;

  /// What the reader view shows, preferring scraped full text over the feed's
  /// own (often truncated) content.
  String get readable =>
      fullContent.isNotEmpty ? fullContent : (content.isNotEmpty ? content : summary);

  factory Article.fromJson(Map<String, dynamic> j) => Article(
        id: _int(j['id']),
        feedUrl: _str(j['feed_url']),
        feedTitle: _str(j['feed_title']),
        feedFaviconUrl: _str(j['feed_favicon_url']),
        title: _str(j['title']),
        url: _str(j['url']),
        author: _str(j['author']),
        summary: _str(j['summary']),
        content: _str(j['content']),
        fullContent: _str(j['full_content']),
        published: _time(j['published']),
        updated: _time(j['updated']),
        isRead: _bool(j['is_read']),
        likeCount: _int(j['like_count']),
        hasLiked: _bool(j['has_liked']),
      );

  Article copyWith({bool? isRead, bool? hasLiked, int? likeCount, String? fullContent}) =>
      Article(
        id: id,
        feedUrl: feedUrl,
        feedTitle: feedTitle,
        feedFaviconUrl: feedFaviconUrl,
        title: title,
        url: url,
        author: author,
        summary: summary,
        content: content,
        fullContent: fullContent ?? this.fullContent,
        published: published,
        updated: updated,
        isRead: isRead ?? this.isRead,
        likeCount: likeCount ?? this.likeCount,
        hasLiked: hasLiked ?? this.hasLiked,
      );
}

class Feed {
  const Feed({
    required this.feedUrl,
    required this.title,
    required this.siteUrl,
    required this.description,
    required this.feedType,
    required this.faviconUrl,
    required this.subscriberCount,
    required this.errorCount,
    required this.lastError,
    required this.lastFetchedAt,
  });

  final String feedUrl;
  final String title;
  final String siteUrl;
  final String description;
  final String feedType;
  final String faviconUrl;
  final int subscriberCount;
  final int errorCount;
  final String lastError;
  final DateTime? lastFetchedAt;

  factory Feed.fromJson(Map<String, dynamic> j) => Feed(
        feedUrl: _str(j['feed_url']),
        title: _str(j['title']),
        siteUrl: _str(j['site_url']),
        description: _str(j['description']),
        feedType: _str(j['feed_type']),
        faviconUrl: _str(j['favicon_url']),
        subscriberCount: _int(j['subscriber_count']),
        errorCount: _int(j['error_count']),
        lastError: _str(j['last_error']),
        lastFetchedAt: _time(j['last_fetched_at']),
      );
}

class Subscription {
  const Subscription({
    required this.id,
    required this.feedUrl,
    required this.feedTitle,
    required this.category,
    required this.addedAt,
    required this.unreadCount,
    required this.faviconUrl,
  });

  final int id;
  final String feedUrl;
  final String feedTitle;
  final String category;
  final DateTime? addedAt;
  final int unreadCount;
  final String faviconUrl;

  factory Subscription.fromJson(Map<String, dynamic> j) => Subscription(
        id: _int(j['id']),
        feedUrl: _str(j['feed_url']),
        feedTitle: _str(j['feed_title']),
        category: _str(j['category']),
        addedAt: _time(j['added_at']),
        unreadCount: _int(j['unread_count']),
        faviconUrl: _str(j['favicon_url']),
      );
}

class Annotation {
  const Annotation({
    required this.id,
    required this.authorDid,
    required this.authorHandle,
    required this.feedUrl,
    required this.articleUrl,
    required this.articleId,
    required this.quote,
    required this.note,
    required this.tags,
    required this.rating,
    required this.createdAt,
  });

  final int id;
  final String authorDid;
  final String authorHandle;
  final String feedUrl;
  final String articleUrl;
  final int? articleId;
  final String quote;
  final String note;
  final List<String> tags;
  final int? rating;
  final DateTime? createdAt;

  factory Annotation.fromJson(Map<String, dynamic> j) => Annotation(
        id: _int(j['id']),
        authorDid: _str(j['author_did']),
        authorHandle: _str(j['author_handle']),
        feedUrl: _str(j['feed_url']),
        articleUrl: _str(j['article_url']),
        articleId: (j['article_id'] as num?)?.toInt(),
        quote: _str(j['quote']),
        note: _str(j['note']),
        tags: _strList(j['tags']),
        rating: (j['rating'] as num?)?.toInt(),
        createdAt: _time(j['created_at']),
      );
}

class TrendingItem {
  const TrendingItem({
    required this.articleId,
    required this.title,
    required this.url,
    required this.author,
    required this.summary,
    required this.feedUrl,
    required this.feedTitle,
    required this.faviconUrl,
    required this.likeCount,
    required this.annotationCount,
    required this.hasLiked,
  });

  final int articleId;
  final String title;
  final String url;
  final String author;
  final String summary;
  final String feedUrl;
  final String feedTitle;
  final String faviconUrl;
  final int likeCount;
  final int annotationCount;
  final bool hasLiked;

  factory TrendingItem.fromJson(Map<String, dynamic> j) => TrendingItem(
        articleId: _int(j['article_id']),
        title: _str(j['title']),
        url: _str(j['url']),
        author: _str(j['author']),
        summary: _str(j['summary']),
        feedUrl: _str(j['feed_url']),
        feedTitle: _str(j['feed_title']),
        faviconUrl: _str(j['favicon_url']),
        likeCount: _int(j['like_count']),
        annotationCount: _int(j['annotation_count']),
        hasLiked: _bool(j['has_liked']),
      );

  TrendingItem copyWith({bool? hasLiked, int? likeCount}) => TrendingItem(
        articleId: articleId,
        title: title,
        url: url,
        author: author,
        summary: summary,
        feedUrl: feedUrl,
        feedTitle: feedTitle,
        faviconUrl: faviconUrl,
        likeCount: likeCount ?? this.likeCount,
        annotationCount: annotationCount,
        hasLiked: hasLiked ?? this.hasLiked,
      );
}

class FeedRecommendation {
  const FeedRecommendation({
    required this.feedUrl,
    required this.title,
    required this.siteUrl,
    required this.description,
    required this.subscriberCount,
    required this.faviconUrl,
    required this.score,
  });

  final String feedUrl;
  final String title;
  final String siteUrl;
  final String description;
  final int subscriberCount;
  final String faviconUrl;
  final double score;

  factory FeedRecommendation.fromJson(Map<String, dynamic> j) => FeedRecommendation(
        feedUrl: _str(j['feed_url']),
        title: _str(j['title']),
        siteUrl: _str(j['site_url']),
        description: _str(j['description']),
        subscriberCount: _int(j['subscriber_count']),
        faviconUrl: _str(j['favicon_url']),
        score: _dbl(j['score']),
      );
}

class PersonRecommendation {
  const PersonRecommendation({
    required this.did,
    required this.handle,
    required this.displayName,
    required this.avatarUrl,
    required this.commonFeeds,
    required this.commonLikes,
    required this.commonTags,
    required this.isFollowed,
    required this.score,
  });

  final String did;
  final String handle;
  final String displayName;
  final String avatarUrl;
  final int commonFeeds;
  final int commonLikes;
  final int commonTags;
  final bool isFollowed;
  final double score;

  String get label => displayName.isNotEmpty ? displayName : handle;

  factory PersonRecommendation.fromJson(Map<String, dynamic> j) => PersonRecommendation(
        did: _str(j['did']),
        handle: _str(j['handle']),
        displayName: _str(j['display_name']),
        avatarUrl: _str(j['avatar_url']),
        commonFeeds: _int(j['common_feeds']),
        commonLikes: _int(j['common_likes']),
        commonTags: _int(j['common_tags']),
        isFollowed: _bool(j['is_followed']),
        score: _dbl(j['score']),
      );
}

class Pagination {
  const Pagination({
    required this.page,
    required this.pageSize,
    required this.hasPrev,
    required this.hasNext,
    required this.prevPage,
    required this.nextPage,
  });

  final int page;
  final int pageSize;
  final bool hasPrev;
  final bool hasNext;
  final int prevPage;
  final int nextPage;

  static const first =
      Pagination(page: 1, pageSize: 25, hasPrev: false, hasNext: false, prevPage: 0, nextPage: 0);

  factory Pagination.fromJson(Map<String, dynamic>? j) => j == null
      ? first
      : Pagination(
          page: _int(j['page']),
          pageSize: _int(j['page_size']),
          hasPrev: _bool(j['has_prev']),
          hasNext: _bool(j['has_next']),
          prevPage: _int(j['prev_page']),
          nextPage: _int(j['next_page']),
        );
}

class Language {
  const Language({required this.code, required this.name});

  final String code;
  final String name;

  factory Language.fromJson(Map<String, dynamic> j) =>
      Language(code: _str(j['code']), name: _str(j['name']));
}

/// Helpers re-exported for the response envelopes in api/responses.dart.
List<T> jsonList<T>(Object? v, T Function(Map<String, dynamic>) f) => _list(v, f);
List<String> jsonStrList(Object? v) => _strList(v);
String jsonStr(Object? v) => _str(v);
int jsonInt(Object? v) => _int(v);
bool jsonBool(Object? v) => _bool(v);
