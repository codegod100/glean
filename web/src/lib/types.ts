export interface User {
	did: string;
	handle: string;
	display_name: string;
	avatar_url: string;
}

export interface Pagination {
	page: number;
	page_size: number;
	has_prev: boolean;
	has_next: boolean;
	prev_page: number;
	next_page: number;
}

export interface Article {
	id: number;
	feed_url: string;
	feed_title: string;
	feed_favicon_url: string;
	title: string;
	url: string;
	author: string;
	summary: string;
	content: string;
	full_content: string;
	published: string | null;
	updated: string | null;
	is_read: boolean;
	like_count: number;
	has_liked: boolean;
}

export interface Feed {
	feed_url: string;
	title: string;
	site_url: string;
	description: string;
	feed_type: string;
	favicon_url: string;
	subscriber_count: number;
	error_count: number;
	last_error: string;
	last_fetched_at: string | null;
}

export interface Subscription {
	id: number;
	feed_url: string;
	feed_title: string;
	category: string;
	added_at: string | null;
	unread_count: number;
	favicon_url: string;
}

export interface Annotation {
	id: number;
	author_did: string;
	author_handle: string;
	feed_url: string;
	article_url: string;
	article_id: number | null;
	quote: string;
	note: string;
	tags: string[];
	rating: number | null;
	created_at: string | null;
}

export interface TrendingItem {
	article_id: number;
	title: string;
	url: string;
	author: string;
	summary: string;
	feed_url: string;
	feed_title: string;
	favicon_url: string;
	like_count: number;
	annotation_count: number;
	has_liked: boolean;
}

export interface FeedRecommendation {
	feed_url: string;
	title: string;
	site_url: string;
	description: string;
	subscriber_count: number;
	favicon_url: string;
	score: number;
}

export interface PersonRecommendation {
	did: string;
	handle: string;
	display_name: string;
	avatar_url: string;
	common_feeds: number;
	common_likes: number;
	common_tags: number;
	is_followed: boolean;
	score: number;
}

export interface Digest {
	title: string;
	summary: string;
	excerpt: string;
	article_ids: number[];
	generated_at: number;
	consumed: boolean;
}

export interface KnownLanguage {
	code: string;
	name: string;
}

export interface MeResponse {
	user: User | null;
	csrf_token: string;
	has_llm: boolean;
	client_id: string;
}

export interface Actor {
	did: string;
	handle: string;
	displayName?: string;
	avatar?: string;
}

export interface MetricFamily {
	name: string;
	type: string;
	description: string;
	labels: Record<string, string> | null;
	value: number;
}
