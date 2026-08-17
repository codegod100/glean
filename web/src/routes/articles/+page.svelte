<script lang="ts">
    import type { PageData } from "./$types";
    import { goto, invalidateAll } from "$app/navigation";
    import ArticleCard from "$lib/components/ArticleCard.svelte";
    import Favicon from "$lib/components/Favicon.svelte";
    import Pagination from "$lib/components/Pagination.svelte";
    import EmptyState from "$lib/components/EmptyState.svelte";
    import Icon from "$lib/components/Icon.svelte";
    import NewArticlesBanner from "$lib/components/NewArticlesBanner.svelte";
    import Seo from "$lib/components/Seo.svelte";
    import { endpoints } from "$lib/api";

    let { data }: { data: PageData } = $props();

    {
        /* svelte-ignore state_referenced_locally -- controlled input, synced on navigate */
    }
    let search = $state(data.search_query);
    let searchTimer: ReturnType<typeof setTimeout>;

    const status = $derived(data.status);
    const sort = $derived(data.sort_oldest ? "oldest" : "");
    const feedURL = $derived(data.feed_url);
    const category = $derived(data.category);

    function buildURL(overrides: Record<string, string | undefined>): string {
        const p = new URLSearchParams();
        const base: Record<string, string> = {
            feed: feedURL,
            status: status,
            q: data.search_query,
            sort: sort,
            category: category,
        };
        const merged = { ...base, ...overrides };
        for (const [k, v] of Object.entries(merged)) {
            if (v) p.set(k, v);
        }
        return "/articles" + (p.toString() ? "?" + p.toString() : "");
    }

    function onSearch(value: string) {
        search = value;
        clearTimeout(searchTimer);
        searchTimer = setTimeout(() => {
            goto(buildURL({ q: value || undefined, page: undefined }), {
                keepFocus: true,
            });
        }, 300);
    }

    function markAllRead() {
        endpoints.markAllRead(feedURL).then(() => invalidateAll());
    }

    // Re-run the route's load function so newly-fetched articles render
    // without a navigation or hard refresh.
    function onrefresh() {
        return invalidateAll();
    }

    // Expanded view: mark articles read on scroll.
    function readOnScroll(node: HTMLElement, id: number) {
        if (!data.expanded_view) return;
        let timer: ReturnType<typeof setTimeout> | undefined;
        const observer = new IntersectionObserver(
            (entries) => {
                for (const entry of entries) {
                    if (entry.isIntersecting) {
                        timer = setTimeout(() => {
                            endpoints.markRead(id).catch(() => {});
                            node.classList.remove("read-marker");
                        }, 3000);
                    } else if (timer) {
                        clearTimeout(timer);
                        timer = undefined;
                    }
                }
            },
            { rootMargin: "0px 0px -50% 0px", threshold: 0 },
        );
        observer.observe(node);
        return {
            destroy() {
                if (timer) clearTimeout(timer);
                observer.disconnect();
            },
        };
    }
</script>

<Seo title={data.feed?.title || "Articles"} />

{#if data.feed}
    <!-- Feed header -->
    <div class="mb-6">
        <a
            href="/articles"
            class="inline-flex items-center gap-1.5 text-[0.7rem] font-bold uppercase tracking-widest text-[var(--muted)] hover:text-[var(--fg)]"
        >
            <Icon name="arrowLeft" class="h-3.5 w-3.5" />All articles
        </a>
        <div
            class="mt-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
        >
            <div class="flex min-w-0 items-center gap-3.5">
                <Favicon src={data.feed.favicon_url} size="h-10 w-10" />
                <div class="min-w-0">
                    <h1
                        class="truncate text-2xl font-extrabold uppercase tracking-tight"
                    >
                        {#if data.feed.site_url}
                            <a
                                href={data.feed.site_url}
                                target="_blank"
                                rel="noopener noreferrer"
                                class="hover:text-[var(--accent)]"
                                >{data.feed.title || data.feed.feed_url}</a
                            >
                        {:else}
                            {data.feed.title || data.feed.feed_url}
                        {/if}
                    </h1>
                    {#if data.feed.site_url}
                        <a
                            href={data.feed.site_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            class="text-xs text-[var(--muted)] hover:text-[var(--accent)]"
                            >{data.feed.site_url}</a
                        >
                    {/if}
                </div>
            </div>
            <div class="flex shrink-0 items-center gap-2">
                {#if !data.is_subscribed}
                    <button
                        onclick={() =>
                            endpoints
                                .addFeed(feedURL)
                                .then(() => invalidateAll())}
                        class="btn btn-accent">Subscribe</button
                    >
                {:else}
                    <button onclick={markAllRead} class="btn"
                        >Mark all read</button
                    >
                {/if}
            </div>
        </div>
        {#if data.feed.description}
            <p class="mt-3 text-sm leading-relaxed text-[var(--muted)]">
                {data.feed.description}
            </p>
        {/if}
    </div>

    <!-- Search + filters (feed view) -->
    <div class="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div class="relative w-full sm:min-w-[200px] sm:flex-1">
            <Icon
                name="search"
                class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--muted)]"
            />
            <input
                type="text"
                value={search}
                oninput={(e) => onSearch(e.currentTarget.value)}
                placeholder="Search articles..."
                class="input-brutal pl-10"
            />
        </div>
        <div class="flex shrink-0 items-center gap-1.5">
            <a
                href={buildURL({ status: "all", page: undefined })}
                class="chip"
                data-active={status === "all"}>All</a
            >
            <a
                href={buildURL({ status: "unread", page: undefined })}
                class="chip"
                data-active={status === "unread"}>Unread</a
            >
            <a
                href={buildURL({ status: "read", page: undefined })}
                class="chip"
                data-active={status === "read"}>Read</a
            >
            <a
                href={buildURL({
                    sort: sort ? undefined : "oldest",
                    page: undefined,
                })}
                class="chip"
                data-active={!!sort}
                title={sort ? "Newest first" : "Oldest first"}
            >
                <Icon
                    name={sort ? "chevronUp" : "chevronDown"}
                    class="h-3.5 w-3.5"
                />
            </a>
        </div>
    </div>
{:else}
    <!-- Title row -->
    <div
        class="mb-4 flex flex-wrap items-center justify-between gap-3 border-b-2 border-[var(--border)] pb-4"
    >
        <div class="flex items-center gap-3">
            <h1 class="text-2xl font-extrabold uppercase tracking-tight">
                Articles
            </h1>
            <button onclick={markAllRead} class="btn">Mark all read</button>
        </div>
        <div class="flex items-center gap-1.5">
            <a
                href={buildURL({ status: "all", page: undefined })}
                class="chip"
                data-active={status === "all"}>All</a
            >
            <a
                href={buildURL({ status: "unread", page: undefined })}
                class="chip"
                data-active={status === "unread" || status === ""}>Unread</a
            >
            <a
                href={buildURL({ status: "read", page: undefined })}
                class="chip"
                data-active={status === "read"}>Read</a
            >
        </div>
    </div>
    <p class="mb-6 text-xs text-[var(--muted)]">
        All your subscribed articles, {sort ? "oldest first" : "newest first"}.
    </p>

    <!-- Sort + categories + search -->
    <div class="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div class="flex flex-wrap items-center gap-1.5">
            <a
                href={buildURL({
                    sort: sort ? undefined : "oldest",
                    page: undefined,
                })}
                class="chip"
                data-active={!!sort}
                title={sort ? "Newest first" : "Oldest first"}
            >
                <Icon
                    name={sort ? "chevronUp" : "chevronDown"}
                    class="h-3.5 w-3.5"
                />
            </a>
            {#if data.categories.length}
                <span class="text-[var(--faint)]">|</span>
                <a
                    href={buildURL({ category: undefined, page: undefined })}
                    class="chip"
                    data-active={!category}>All</a
                >
                {#each data.categories as cat}
                    <a
                        href={buildURL({ category: cat, page: undefined })}
                        class="chip"
                        data-active={category === cat}>{cat}</a
                    >
                {/each}
                <a
                    href={buildURL({ category: "__none__", page: undefined })}
                    class="chip"
                    data-active={category === "__none__"}>Uncategorized</a
                >
            {/if}
        </div>
        <div class="relative w-full sm:min-w-[180px] sm:flex-1">
            <Icon
                name="search"
                class="pointer-events-none absolute left-2.5 top-1/2 h-3 w-3 -translate-y-1/2 text-[var(--muted)]"
            />
            <input
                type="text"
                value={search}
                oninput={(e) => onSearch(e.currentTarget.value)}
                placeholder="Search articles..."
                class="input-brutal px-3 py-1.5 pl-8 text-xs"
            />
        </div>
    </div>
{/if}

<!-- Live banner: surfaces newly-fetched articles since the page loaded -->
<NewArticlesBanner since={data.now} {onrefresh} />

<!-- List -->
<div class="space-y-3">
    {#if data.articles.length === 0}
        <EmptyState
            icon="feed"
            title="No articles found"
            subtitle={status === "unread"
                ? "You've read everything. Nice work."
                : status === "read"
                  ? "Nothing marked as read yet."
                  : "Subscribe to feeds to see articles here."}
        />
    {:else}
        {#each data.articles as a (a.id)}
            <div use:readOnScroll={a.id} class="contents">
                <ArticleCard
                    article={a}
                    expanded={data.expanded_view}
                    navSuffix="?liked=1"
                />
            </div>
        {/each}
    {/if}
</div>

<Pagination
    page={data.pagination}
    base="/articles"
    params={{ feed: feedURL, status, q: data.search_query, sort, category }}
/>
