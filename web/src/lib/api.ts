import type {
  Actor,
  Annotation,
  Article,
  Digest,
  Feed,
  FeedRecommendation,
  KnownLanguage,
  MeResponse,
  MetricFamily,
  Pagination,
  PersonRecommendation,
  Subscription,
  TrendingItem,
  User,
} from "./types";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

let cachedCsrf = "";

export function setCsrfToken(token: string) {
  cachedCsrf = token;
}

function readCsrfToken(): string {
  if (cachedCsrf) return cachedCsrf;
  if (typeof document !== "undefined") {
    const match = document.cookie.match(/(?:^|;\s*)glean_csrf=([^;]+)/);
    if (match) return decodeURIComponent(match[1]);
  }
  return "";
}

type FetchFn = typeof fetch;

// Encode a body for the Go handlers, which read inputs via r.FormValue.
function encodeBody(
  body: Record<string, unknown> | FormData | undefined,
): { body: BodyInit | undefined; contentType?: string } {
  if (!body) return { body: undefined };
  if (body instanceof FormData) return { body };
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(body)) {
    if (Array.isArray(v)) {
      for (const item of v) params.append(k, String(item));
    } else if (v !== undefined && v !== null) {
      params.set(k, String(v));
    }
  }
  return { body: params, contentType: "application/x-www-form-urlencoded" };
}

async function request<T>(
  method: string,
  path: string,
  fetchFn: FetchFn,
  opts: { body?: Record<string, unknown> | FormData; query?: Record<string, string> } = {},
): Promise<T> {
  const search = new URLSearchParams();
  if (opts.query) {
    for (const [k, v] of Object.entries(opts.query)) {
      if (v !== "" && v != null) search.set(k, v);
    }
  }
  const qs = search.toString();
  const { body, contentType } = encodeBody(opts.body);

  const headers: Record<string, string> = {};
  if (contentType) headers["Content-Type"] = contentType;
  if (method !== "GET" && method !== "HEAD") {
    const token = readCsrfToken();
    if (token) headers["X-CSRF-Token"] = token;
  }

  const res = await fetchFn(`/api${path}${qs ? `?${qs}` : ""}`, {
    method,
    headers,
    body,
    credentials: "same-origin",
  });

  if (res.status === 204) return undefined as T;

  if (!res.ok) {
    let message = res.statusText;
    try {
      const data = await res.json();
      message = data.error ?? message;
    } catch {
      // keep statusText
    }
    throw new ApiError(res.status, message);
  }

  const ct = res.headers.get("content-type") ?? "";
  if (ct.includes("application/json")) return (await res.json()) as T;
  return (await res.text()) as unknown as T;
}

// Typed endpoint helpers.

export interface DashboardData {
  user: User;
  subscription_count: number;
  unread_count: number;
  articles: Article[];
  personal_trending: TrendingItem[];
  global_trending: TrendingItem[];
  digest_enabled: boolean;
  has_llm: boolean;
  now: number;
}

export interface ArticlesData {
  user: User;
  articles: Article[];
  feed_url: string;
  status: string;
  search_query: string;
  sort_oldest: boolean;
  category: string;
  categories: string[];
  expanded_view: boolean;
  pagination: Pagination;
  now: number;
  feed?: Feed;
  is_subscribed?: boolean;
}

export interface ArticleDetailData {
  user: User;
  current_user_did: string;
  article: Article;
  feed: Feed;
  annotations: Annotation[];
  next_id: number | null;
  next_suffix: string;
}

export interface FeedsData {
  user: User;
  subscriptions: Subscription[];
  subscription_count: number;
  categories: string[];
  category: string;
  dead_feeds: Feed[];
  pagination: Pagination;
}

export interface LibraryData {
  user: User;
  articles: Article[];
  annotations: Annotation[];
  liked_page: Pagination;
  annot_page: Pagination;
}

export interface TrendingData {
  user: User | null;
  trending: TrendingItem[];
  scope: string;
  pagination: Pagination;
}

export interface ProfileData {
  user: User;
  profile_user: User;
  subscriptions: Subscription[];
  annotations: Annotation[];
  subscription_count: number;
  annotation_count: number;
  user_languages: string[];
  available_languages: KnownLanguage[];
  expanded_view: boolean;
  digest_enabled: boolean;
}

export interface StatsData {
  user: User | null;
  metrics: Record<string, MetricFamily[]>;
}

export interface PeopleRecs {
  followed: PersonRecommendation[];
  discover: PersonRecommendation[];
}

export interface FeedRecs {
  feeds: FeedRecommendation[];
  subscription_count: number;
}

type Endpoints = ReturnType<typeof createEndpoints>;

function createEndpoints(fetchFn: FetchFn) {
  const get = <T>(path: string, query?: Record<string, string>) =>
    request<T>("GET", path, fetchFn, { query });
  const post = <T>(path: string, body?: Record<string, unknown> | FormData) =>
    request<T>("POST", path, fetchFn, { body });
  const del = <T>(path: string, body?: Record<string, unknown>) =>
    request<T>("DELETE", path, fetchFn, { body });

  return {
    me: () => get<MeResponse>("/me"),
    dashboard: () => get<DashboardData>("/dashboard"),
    articles: (params: Record<string, string>) => get<ArticlesData>("/articles", params),
    article: (id: number, params: Record<string, string>) =>
      get<ArticleDetailData>(`/articles/${id}`, params),
    newArticleCount: (since: number) =>
      get<{ count: number }>("/articles/new-count", { since: String(since) }),
    markRead: (id: number) => post<{ id: number; is_read: boolean }>(`/articles/${id}/read`),
    markUnread: (id: number) =>
      post<{ id: number; is_read: boolean }>(`/articles/${id}/unread`),
    toggleLike: (id: number) =>
      post<{ id: number; liked: boolean; like_count: number }>(`/articles/${id}/like`),
    fetchContent: (id: number) =>
      post<{ id: number; full_content: string }>(`/articles/${id}/fetch-content`),
    markAllRead: (feed?: string) =>
      post<void>("/articles/mark-all-read", feed ? { feed } : {}),

    feeds: (category?: string) => get<FeedsData>("/feeds", category ? { category } : {}),
    addFeed: (feed_url: string, category?: string) =>
      post<{ subscription: Subscription }>("/feeds/add", { feed_url, category }),
    editFeed: (feed_url: string, category: string) =>
      post<{ subscription: Subscription }>("/feeds/edit", { feed_url, category }),
    removeFeed: (url: string) => del<void>("/feeds/remove", { url }),
    refreshFeeds: (category?: string) =>
      post<{ subscriptions: Subscription[] }>("/feeds/refresh", category ? { category } : {}),
    retryFeed: (url: string) => post<{ dead_feeds: Feed[] }>("/feeds/retry", { url }),
    clearFeeds: () => post<void>("/feeds/clear", {}),
    uploadOpml: (form: FormData) => post<{ added: number }>("/feeds/opml/upload", form),

    trending: (params: Record<string, string>) => get<TrendingData>("/trending", params),

    library: (params: Record<string, string>) => get<LibraryData>("/library", params),
    createAnnotation: (body: Record<string, unknown>) =>
      post<{ annotation: Annotation }>("/library/create", body),
    deleteAnnotation: (id: number) => post<void>(`/library/${id}/delete`),

    profile: (did: string) => get<ProfileData>(`/profile/${did}`),

    articleRecs: () => get<{ articles: Article[] }>("/recs/articles"),
    feedRecs: () => get<FeedRecs>("/recs/feeds"),
    peopleRecs: () => get<PeopleRecs>("/recs/people"),
    dismissFeed: (feed_url: string) => post<void>("/recs/dismiss-feed", { feed_url }),
    dismissArticle: (article_url: string) =>
      post<void>("/recs/dismiss-article", { article_url }),
    dismissPerson: (target_did: string) =>
      post<void>("/recs/dismiss-person", { target_did }),

    toggleLanguage: (code: string) =>
      post<{ languages: string[] }>(`/settings/languages/${code}`),
    toggleExpandedView: (expanded_view: boolean) =>
      post<{ expanded_view: boolean }>("/settings/expanded-view", {
        expanded_view: expanded_view ? "1" : "0",
      }),
    toggleDigest: (digest_enabled: boolean) =>
      post<{ digest_enabled: boolean }>("/settings/digest-enabled", {
        digest_enabled: digest_enabled ? "1" : "0",
      }),

    digest: () => get<Digest | null>("/digest"),
    markDigestRead: (ids: number[]) =>
      post<Digest>("/digest/mark-read", { ids: ids.map(String) }),

    authActors: (q: string) => get<{ actors: Actor[] }>("/auth/actors", { q }),
    authStart: (handle: string) => post<{ redirect: string }>("/auth/start", { handle }),
    authRegister: () => get<{ redirect: string }>("/auth/register"),
    authLogout: () => post<{ redirect: string }>("/auth/logout"),

    stats: () => get<StatsData>("/stats"),
  };
}

// Default endpoints for browser use (uses global fetch).
export const endpoints: Endpoints = createEndpoints(fetch);

/** Build endpoints bound to a specific fetch (e.g. SvelteKit event.fetch for SSR). */
export function endpointsFor(fetchFn: FetchFn): Endpoints {
  return createEndpoints(fetchFn);
}
