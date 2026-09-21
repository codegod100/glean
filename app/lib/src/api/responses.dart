import '../models/models.dart';

/// Response envelopes for each endpoint, mirroring the unexported structs at
/// the bottom of internal/server/api.go.

class MeResponse {
  const MeResponse({this.user, required this.csrfToken, required this.hasLlm, required this.clientId});

  final User? user;
  final String csrfToken;
  final bool hasLlm;
  final String clientId;

  bool get signedIn => user != null;

  /// True when the server is running without a real OAuth client id, in which
  /// case sign-in cannot complete (see GLEAN_OAUTH_CLIENT_ID).
  bool get oauthConfigured => clientId.isNotEmpty;

  factory MeResponse.fromJson(Map<String, dynamic> j) => MeResponse(
        user: j['user'] == null ? null : User.fromJson(j['user'] as Map<String, dynamic>),
        csrfToken: jsonStr(j['csrf_token']),
        hasLlm: jsonBool(j['has_llm']),
        clientId: jsonStr(j['client_id']),
      );
}

class DashboardResponse {
  const DashboardResponse({
    required this.user,
    required this.subscriptionCount,
    required this.unreadCount,
    required this.articles,
    required this.personalTrending,
    required this.globalTrending,
    required this.digestEnabled,
    required this.hasLlm,
  });

  final User user;
  final int subscriptionCount;
  final int unreadCount;
  final List<Article> articles;
  final List<TrendingItem> personalTrending;
  final List<TrendingItem> globalTrending;
  final bool digestEnabled;
  final bool hasLlm;

  factory DashboardResponse.fromJson(Map<String, dynamic> j) => DashboardResponse(
        user: User.fromJson((j['user'] as Map<String, dynamic>?) ?? const {}),
        subscriptionCount: jsonInt(j['subscription_count']),
        unreadCount: jsonInt(j['unread_count']),
        articles: jsonList(j['articles'], Article.fromJson),
        personalTrending: jsonList(j['personal_trending'], TrendingItem.fromJson),
        globalTrending: jsonList(j['global_trending'], TrendingItem.fromJson),
        digestEnabled: jsonBool(j['digest_enabled']),
        hasLlm: jsonBool(j['has_llm']),
      );
}

class ArticlesResponse {
  const ArticlesResponse({
    required this.articles,
    required this.feedUrl,
    required this.status,
    required this.searchQuery,
    required this.sortOldest,
    required this.category,
    required this.categories,
    required this.expandedView,
    required this.pagination,
    this.feed,
    required this.isSubscribed,
  });

  final List<Article> articles;
  final String feedUrl;
  final String status;
  final String searchQuery;
  final bool sortOldest;
  final String category;
  final List<String> categories;
  final bool expandedView;
  final Pagination pagination;
  final Feed? feed;
  final bool isSubscribed;

  factory ArticlesResponse.fromJson(Map<String, dynamic> j) => ArticlesResponse(
        articles: jsonList(j['articles'], Article.fromJson),
        feedUrl: jsonStr(j['feed_url']),
        status: jsonStr(j['status']),
        searchQuery: jsonStr(j['search_query']),
        sortOldest: jsonBool(j['sort_oldest']),
        category: jsonStr(j['category']),
        categories: jsonStrList(j['categories']),
        expandedView: jsonBool(j['expanded_view']),
        pagination: Pagination.fromJson(j['pagination'] as Map<String, dynamic>?),
        feed: j['feed'] == null ? null : Feed.fromJson(j['feed'] as Map<String, dynamic>),
        isSubscribed: jsonBool(j['is_subscribed']),
      );
}

class ArticleDetailResponse {
  const ArticleDetailResponse({
    required this.currentUserDid,
    required this.article,
    required this.feed,
    required this.annotations,
    this.nextId,
  });

  final String currentUserDid;
  final Article article;
  final Feed feed;
  final List<Annotation> annotations;
  final int? nextId;

  factory ArticleDetailResponse.fromJson(Map<String, dynamic> j) => ArticleDetailResponse(
        currentUserDid: jsonStr(j['current_user_did']),
        article: Article.fromJson((j['article'] as Map<String, dynamic>?) ?? const {}),
        feed: Feed.fromJson((j['feed'] as Map<String, dynamic>?) ?? const {}),
        annotations: jsonList(j['annotations'], Annotation.fromJson),
        nextId: (j['next_id'] as num?)?.toInt(),
      );
}

class FeedsResponse {
  const FeedsResponse({
    required this.subscriptions,
    required this.subscriptionCount,
    required this.categories,
    required this.category,
    required this.deadFeeds,
    required this.pagination,
  });

  final List<Subscription> subscriptions;
  final int subscriptionCount;
  final List<String> categories;
  final String category;
  final List<Feed> deadFeeds;
  final Pagination pagination;

  factory FeedsResponse.fromJson(Map<String, dynamic> j) => FeedsResponse(
        subscriptions: jsonList(j['subscriptions'], Subscription.fromJson),
        subscriptionCount: jsonInt(j['subscription_count']),
        categories: jsonStrList(j['categories']),
        category: jsonStr(j['category']),
        deadFeeds: jsonList(j['dead_feeds'], Feed.fromJson),
        pagination: Pagination.fromJson(j['pagination'] as Map<String, dynamic>?),
      );
}

class TrendingResponse {
  const TrendingResponse({required this.trending, required this.scope, required this.pagination});

  final List<TrendingItem> trending;
  final String scope;
  final Pagination pagination;

  factory TrendingResponse.fromJson(Map<String, dynamic> j) => TrendingResponse(
        trending: jsonList(j['trending'], TrendingItem.fromJson),
        scope: jsonStr(j['scope']),
        pagination: Pagination.fromJson(j['pagination'] as Map<String, dynamic>?),
      );
}

class LibraryResponse {
  const LibraryResponse({
    required this.articles,
    required this.annotations,
    required this.likedPage,
    required this.annotPage,
  });

  final List<Article> articles;
  final List<Annotation> annotations;
  final Pagination likedPage;
  final Pagination annotPage;

  factory LibraryResponse.fromJson(Map<String, dynamic> j) => LibraryResponse(
        articles: jsonList(j['articles'], Article.fromJson),
        annotations: jsonList(j['annotations'], Annotation.fromJson),
        likedPage: Pagination.fromJson(j['liked_page'] as Map<String, dynamic>?),
        annotPage: Pagination.fromJson(j['annot_page'] as Map<String, dynamic>?),
      );
}

class ProfileResponse {
  const ProfileResponse({
    required this.profileUser,
    required this.subscriptions,
    required this.annotations,
    required this.subscriptionCount,
    required this.annotationCount,
    required this.userLanguages,
    required this.availableLanguages,
    required this.expandedView,
    required this.digestEnabled,
  });

  final User profileUser;
  final List<Subscription> subscriptions;
  final List<Annotation> annotations;
  final int subscriptionCount;
  final int annotationCount;
  final List<String> userLanguages;
  final List<Language> availableLanguages;
  final bool expandedView;
  final bool digestEnabled;

  factory ProfileResponse.fromJson(Map<String, dynamic> j) => ProfileResponse(
        profileUser: User.fromJson((j['profile_user'] as Map<String, dynamic>?) ?? const {}),
        subscriptions: jsonList(j['subscriptions'], Subscription.fromJson),
        annotations: jsonList(j['annotations'], Annotation.fromJson),
        subscriptionCount: jsonInt(j['subscription_count']),
        annotationCount: jsonInt(j['annotation_count']),
        userLanguages: jsonStrList(j['user_languages']),
        availableLanguages: jsonList(j['available_languages'], Language.fromJson),
        expandedView: jsonBool(j['expanded_view']),
        digestEnabled: jsonBool(j['digest_enabled']),
      );
}

class PeopleRecsResponse {
  const PeopleRecsResponse({required this.followed, required this.discover});

  final List<PersonRecommendation> followed;
  final List<PersonRecommendation> discover;

  factory PeopleRecsResponse.fromJson(Map<String, dynamic> j) => PeopleRecsResponse(
        followed: jsonList(j['followed'], PersonRecommendation.fromJson),
        discover: jsonList(j['discover'], PersonRecommendation.fromJson),
      );
}

class FeedRecsResponse {
  const FeedRecsResponse({required this.feeds, required this.subscriptionCount});

  final List<FeedRecommendation> feeds;
  final int subscriptionCount;

  factory FeedRecsResponse.fromJson(Map<String, dynamic> j) => FeedRecsResponse(
        feeds: jsonList(j['feeds'], FeedRecommendation.fromJson),
        subscriptionCount: jsonInt(j['subscription_count']),
      );
}

class LikeResult {
  const LikeResult({required this.id, required this.liked, required this.likeCount});

  final int id;
  final bool liked;
  final int likeCount;

  factory LikeResult.fromJson(Map<String, dynamic> j) => LikeResult(
        id: jsonInt(j['id']),
        liked: jsonBool(j['liked']),
        likeCount: jsonInt(j['like_count']),
      );
}
