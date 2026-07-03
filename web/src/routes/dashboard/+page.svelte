<script lang="ts">
    import type { PageData } from "./$types";
    import ArticleCard from "$lib/components/ArticleCard.svelte";
    import TrendingCard from "$lib/components/TrendingCard.svelte";
    import ProfileCard from "$lib/components/ProfileCard.svelte";
    import FeedRecommendationCard from "$lib/components/FeedRecommendationCard.svelte";
    import EmptyState from "$lib/components/EmptyState.svelte";
    import Icon from "$lib/components/Icon.svelte";
    import NewArticlesBanner from "$lib/components/NewArticlesBanner.svelte";
    import { endpoints } from "$lib/api";
    import type {
        Article,
        Digest,
        FeedRecommendation,
        PersonRecommendation,
        TrendingItem,
    } from "$lib/types";

    let { data }: { data: PageData } = $props();

    let articleRecs = $state<Article[] | null>(null);
    let feedRecs = $state<FeedRecommendation[] | null>(null);
    let followed = $state<PersonRecommendation[]>([]);
    let discover = $state<PersonRecommendation[]>([]);
    let digest = $state<Digest | null>(null);
    let locallyRead = $state<Set<number>>(new Set());

    const unreadArticles = $derived(
        data.articles.filter((a) => !locallyRead.has(a.id)),
    );

    $effect(() => {
        if (data.subscription_count > 0) {
            endpoints
                .articleRecs()
                .then((r) => (articleRecs = r.articles))
                .catch(() => (articleRecs = []));
        }
        if (data.digest_enabled && data.has_llm) {
            endpoints
                .digest()
                .then((d) => (digest = d))
                .catch(() => (digest = null));
        }
        endpoints
            .feedRecs()
            .then((r) => (feedRecs = r.feeds))
            .catch(() => (feedRecs = []));
        endpoints
            .peopleRecs()
            .then((r) => {
                followed = r.followed;
                discover = r.discover;
            })
            .catch(() => {});
    });

    function dismissArticle(url: string) {
        articleRecs = (articleRecs ?? []).filter((a) => a.url !== url);
    }
    function dismissFeed(url: string) {
        feedRecs = (feedRecs ?? []).filter((f) => f.feed_url !== url);
    }
    function dismissPerson(did: string) {
        followed = followed.filter((p) => p.did !== did);
        discover = discover.filter((p) => p.did !== did);
    }

    let digestOpen = $state(false);
    async function markDigestRead() {
        if (!digest) return;
        if (!confirm("Mark these articles as read?")) return;
        await endpoints.markDigestRead(digest.article_ids);
        digest = { ...digest, consumed: true };
        locallyRead = new Set(digest.article_ids);
    }

    const trending: TrendingItem[] = $derived([
        ...(data.global_trending ?? []),
        ...(data.personal_trending ?? []),
    ]);
</script>

<!-- Header -->
<div
    class="mb-6 flex flex-wrap items-end justify-between gap-3 border-b-2 border-[var(--border)] pb-4"
>
    <div>
        <h1 class="text-2xl font-extrabold uppercase tracking-tight">
            Dashboard
        </h1>
        <p class="mt-1 text-xs text-[var(--muted)]">
            {data.subscription_count === 0
                ? "Get started by subscribing to RSS feeds."
                : "Your personalized feed, based on your social graph."}
        </p>
    </div>
    {#if data.subscription_count > 0}
        <div class="flex gap-2">
            <span class="chip" data-active="true"
                >{data.unread_count} unread</span
            >
            <span class="chip">{data.subscription_count} feeds</span>
        </div>
    {/if}
</div>

{#if data.subscription_count > 0}
    <NewArticlesBanner since={data.now} />
{/if}

{#if data.subscription_count === 0}
    <div class="mb-10">
        <EmptyState
            icon="plus"
            title="Add your first feed"
            subtitle="Subscribe to RSS feeds to start building your personalized reading experience."
        />
        <div class="mt-4 text-center">
            <a href="/feeds" class="btn btn-accent"
                ><Icon name="plus" class="h-4 w-4" />Add feeds</a
            >
        </div>
    </div>
{:else if unreadArticles.length > 0}
    <!-- Recommended articles -->
    {#if articleRecs && articleRecs.length > 0}
        <section class="mb-10">
            <h2
                class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
            >
                Recommended for you
            </h2>
            <div class="space-y-3">
                {#each articleRecs as a (a.id)}
                    <ArticleCard
                        article={a}
                        dismissible
                        onDismiss={() => dismissArticle(a.url)}
                    />
                {/each}
            </div>
        </section>
    {/if}

    <!-- Digest -->
    {#if data.digest_enabled && data.has_llm}
        {#if digest === null}
            <section class="mb-10">
                <h2
                    class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
                >
                    Daily digest
                </h2>
                <div class="panel animate-pulse px-5 py-4">
                    <div class="h-4 w-2/3 bg-[var(--surface)]"></div>
                </div>
            </section>
        {:else if !digest.consumed}
            <section class="mb-10">
                <h2
                    class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
                >
                    Daily digest
                </h2>
                <article
                    class="panel border-l-[6px] border-l-[var(--accent)] p-5"
                >
                    <div class="flex items-start justify-between gap-4">
                        <div class="min-w-0 flex-1">
                            <button
                                class="w-full text-left"
                                onclick={() => (digestOpen = !digestOpen)}
                            >
                                <span
                                    class="font-extrabold leading-snug hover:text-[var(--accent)]"
                                    >{digest.title}</span
                                >
                            </button>
                            {#if !digestOpen}
                                <p
                                    class="mt-2 line-clamp-2 text-xs leading-relaxed text-[var(--muted)]"
                                >
                                    {digest.excerpt}
                                </p>
                            {/if}
                        </div>
                        <button onclick={markDigestRead} class="btn shrink-0"
                            >Read</button
                        >
                    </div>
                    {#if digestOpen}
                        <div
                            class="article-body mt-3 border-t-2 border-[var(--border)] pt-4 text-sm"
                        >
                            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                            {@html digest.summary}
                        </div>
                    {/if}
                </article>
            </section>
        {/if}
    {/if}

    <!-- Unread -->
    <section class="mb-10">
        <div class="mb-3 flex items-center justify-between">
            <h2
                class="text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
            >
                Unread articles
            </h2>
            <a
                href="/articles"
                class="text-[0.65rem] font-bold uppercase tracking-widest text-[var(--muted)] hover:text-[var(--accent)]"
                >View all</a
            >
        </div>
        <div class="space-y-3">
            {#each unreadArticles as a (a.id)}
                <ArticleCard article={a} />
            {/each}
        </div>
        {#if data.unread_count > 5}
            <a
                href="/articles"
                class="mt-4 block py-2 text-center text-[0.65rem] font-bold uppercase tracking-widest text-[var(--muted)] hover:text-[var(--accent)]"
            >
                View all {data.unread_count} unread articles
            </a>
        {/if}
    </section>
{:else}
    <!-- Caught up: still show recs above the empty state -->
    {#if articleRecs && articleRecs.length > 0}
        <section class="mb-10">
            <h2
                class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
            >
                Recommended for you
            </h2>
            <div class="space-y-3">
                {#each articleRecs as a (a.id)}
                    <ArticleCard
                        article={a}
                        dismissible
                        onDismiss={() => dismissArticle(a.url)}
                    />
                {/each}
            </div>
        </section>
    {/if}
    <section class="mb-10">
        <h2
            class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
        >
            Unread articles
        </h2>
        <EmptyState
            icon="check"
            title="You're all caught up!"
            subtitle="No unread articles. Check back later for new content."
        />
    </section>
{/if}

<!-- Trending -->
{#if trending.length > 0}
    <section class="mb-10">
        <div class="mb-3 flex items-center justify-between">
            <h2
                class="text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
            >
                {data.subscription_count === 0
                    ? "Trending"
                    : "Trending in your network"}
            </h2>
            <a
                href={data.subscription_count === 0
                    ? "/trending"
                    : "/trending?scope=for-me"}
                class="text-[0.65rem] font-bold uppercase tracking-widest text-[var(--muted)] hover:text-[var(--accent)]"
                >See all</a
            >
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
            {#each trending as t (t.article_id)}
                <TrendingCard item={t} />
            {/each}
        </div>
    </section>
{/if}

<!-- People recs -->
{#if followed.length > 0 || discover.length > 0}
    <section class="mb-10 grid grid-cols-1 gap-8 md:grid-cols-2">
        {#if followed.length > 0}
            <div>
                <h2
                    class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
                >
                    Your network
                </h2>
                <div class="space-y-3">
                    {#each followed as p (p.did)}
                        <ProfileCard person={p} onDismiss={dismissPerson} />
                    {/each}
                </div>
            </div>
        {/if}
        {#if discover.length > 0}
            <div>
                <h2
                    class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
                >
                    Discover new readers
                </h2>
                <div class="space-y-3">
                    {#each discover as p (p.did)}
                        <ProfileCard person={p} onDismiss={dismissPerson} />
                    {/each}
                </div>
            </div>
        {/if}
    </section>
{/if}

<!-- Feed recs -->
{#if feedRecs !== null && feedRecs.length > 0}
    <section class="mb-10">
        <h2
            class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
        >
            {data.subscription_count === 0
                ? "Popular feeds to get started"
                : "Recommended feeds"}
        </h2>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
            {#each feedRecs as f (f.feed_url)}
                <FeedRecommendationCard rec={f} onDismiss={dismissFeed} />
            {/each}
        </div>
    </section>
{/if}
