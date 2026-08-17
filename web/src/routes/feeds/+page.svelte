<script lang="ts">
    import { onMount } from "svelte";
    import type { PageData } from "./$types";
    import FeedItem from "$lib/components/FeedItem.svelte";
    import Pagination from "$lib/components/Pagination.svelte";
    import EmptyState from "$lib/components/EmptyState.svelte";
    import FeedRecommendationCard from "$lib/components/FeedRecommendationCard.svelte";
    import Icon from "$lib/components/Icon.svelte";
    import Seo from "$lib/components/Seo.svelte";
    import { endpoints } from "$lib/api";
    import { invalidateAll } from "$app/navigation";
    import type { FeedRecommendation } from "$lib/types";

    let { data }: { data: PageData } = $props();

    let addURL = $state("");
    let addCategory = $state("");
    let addError = $state("");
    let adding = $state(false);

    let refreshing = $state(false);
    let feedRecs = $state<FeedRecommendation[] | null>(null);
    let deadFeeds = $derived(data.dead_feeds);

    onMount(() => {
        endpoints
            .feedRecs()
            .then((r) => (feedRecs = r.feeds))
            .catch(() => (feedRecs = []));
    });

    async function addFeed() {
        adding = true;
        addError = "";
        try {
            await endpoints.addFeed(addURL, addCategory);
            addURL = "";
            addCategory = "";
            await invalidateAll();
        } catch (e) {
            addError = e instanceof Error ? e.message : "Failed";
        } finally {
            adding = false;
        }
    }

    async function refresh() {
        refreshing = true;
        try {
            await endpoints.refreshFeeds(data.category);
            await new Promise((r) => setTimeout(r, 4000));
            await invalidateAll();
        } finally {
            refreshing = false;
        }
    }

    async function retry(url: string) {
        await endpoints.retryFeed(url);
        await invalidateAll();
    }

    async function removeDead(url: string) {
        await endpoints.removeFeed(url);
        await invalidateAll();
    }

    async function clearAll() {
        if (
            !confirm(
                "Are you sure you want to unsubscribe from ALL feeds? This cannot be undone.",
            )
        )
            return;
        await endpoints.clearFeeds();
        await invalidateAll();
    }

    async function uploadOpml(e: Event) {
        const input = e.target as HTMLInputElement;
        const file = input.files?.[0];
        if (!file) return;
        const form = new FormData();
        form.append("opml", file);
        await endpoints.uploadOpml(form);
        await invalidateAll();
    }

    function dismissRec(url: string) {
        feedRecs = (feedRecs ?? []).filter((f) => f.feed_url !== url);
    }
</script>

<Seo title="Discover feeds" />

<div class="flex items-center justify-between gap-3 mb-2 flex-wrap">
    <h1 class="text-2xl font-extrabold uppercase tracking-tight">
        Feeds <span class="text-base font-normal text-[var(--muted)]"
            >({data.subscription_count})</span
        >
    </h1>
    <button onclick={refresh} disabled={refreshing} class="btn">
        <Icon name="refresh" class="h-4 w-4" />
        {refreshing ? "Refreshing..." : "Refresh"}
    </button>
</div>
<p class="text-xs uppercase tracking-widest text-[var(--muted)] mb-6">
    Manage your RSS, Atom, and AT Protocol subscriptions.
</p>

{#if deadFeeds.length > 0}
    <section class="panel mb-6">
        <div
            class="border-b-2 border-[var(--border)] px-4 py-2"
            style="background:var(--danger);color:#fff"
        >
            <span class="text-xs font-extrabold uppercase tracking-widest">
                Feeds with errors ({deadFeeds.length})
            </span>
        </div>
        <div class="divide-y-2 divide-[var(--border)]">
            {#each deadFeeds as f (f.feed_url)}
                <div
                    class="flex items-start justify-between gap-3 p-4 flex-wrap"
                >
                    <div class="min-w-0 flex-1">
                        <span class="block truncate font-bold"
                            >{f.title || f.feed_url}</span
                        >
                        <div class="mt-1 flex items-center gap-2 flex-wrap">
                            <span
                                class="chip"
                                style="background:var(--danger);color:#fff;border-color:var(--danger)"
                            >
                                {f.error_count} ERR
                            </span>
                            {#if f.last_error}
                                <span
                                    class="text-xs text-[var(--muted)] truncate max-w-48"
                                    title={f.last_error}
                                >
                                    {f.last_error}
                                </span>
                            {/if}
                        </div>
                    </div>
                    <div class="flex items-center gap-2 shrink-0">
                        <button onclick={() => retry(f.feed_url)} class="btn"
                            >Retry</button
                        >
                        <button
                            onclick={() => removeDead(f.feed_url)}
                            class="btn"
                            style="background:var(--danger);color:#fff"
                            >Unsub</button
                        >
                    </div>
                </div>
            {/each}
        </div>
    </section>
{/if}

{#if feedRecs !== null && feedRecs.length > 0}
    <section class="mb-6">
        <h2
            class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
        >
            {data.subscription_count === 0
                ? "Popular feeds to get started"
                : "Recommended feeds"}
        </h2>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {#each feedRecs as f (f.feed_url)}
                <FeedRecommendationCard rec={f} onDismiss={dismissRec} />
            {/each}
        </div>
    </section>
{/if}

<div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
    <div class="lg:col-span-2">
        {#if data.subscription_count > 0}
            <div class="mb-4 flex flex-wrap gap-2">
                <a
                    href="/feeds"
                    class="chip panel-press"
                    data-active={!data.category ? "true" : "false"}>All</a
                >
                {#each data.categories as cat (cat)}
                    <a
                        href="/feeds?category={encodeURIComponent(cat)}"
                        class="chip panel-press"
                        data-active={data.category === cat ? "true" : "false"}
                        >{cat}</a
                    >
                {/each}
                <a
                    href="/feeds?category=__none__"
                    class="chip panel-press"
                    data-active={data.category === "__none__"
                        ? "true"
                        : "false"}>Uncategorized</a
                >
            </div>
        {/if}

        <div class="panel divide-y-2 divide-[var(--border)]">
            {#if data.subscriptions.length === 0}
                <EmptyState
                    icon="feed"
                    title="No feeds yet"
                    subtitle="Add one using the form on the right."
                />
            {:else}
                {#each data.subscriptions as sub (sub.id)}
                    <FeedItem {sub} />
                {/each}
            {/if}
        </div>

        <Pagination
            page={data.pagination}
            base="/feeds"
            params={{ category: data.category }}
        />
    </div>

    <aside class="space-y-6">
        <section class="panel p-4 space-y-4">
            <h2
                class="text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
            >
                Add feed
            </h2>
            <form
                onsubmit={(e) => {
                    e.preventDefault();
                    addFeed();
                }}
                class="space-y-3"
            >
                <input
                    bind:value={addURL}
                    type="text"
                    placeholder="https://example.com/feed.xml"
                    class="input-brutal"
                    required
                />
                <input
                    bind:value={addCategory}
                    type="text"
                    placeholder="Category (optional)"
                    class="input-brutal"
                />
                <button
                    type="submit"
                    disabled={adding}
                    class="btn btn-accent w-full"
                >
                    <Icon name="plus" class="h-4 w-4" />
                    Add
                </button>
            </form>
            {#if addError}
                <p
                    class="text-xs font-bold uppercase tracking-wide text-[var(--danger)]"
                >
                    {addError}
                </p>
            {/if}

            <div class="border-t-2 border-[var(--border)] pt-4 space-y-3">
                <h2
                    class="text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
                >
                    Import / Export
                </h2>
                <label class="block cursor-pointer">
                    <input
                        type="file"
                        accept=".opml,.xml"
                        onchange={uploadOpml}
                        class="hidden"
                    />
                    <span class="btn w-full">
                        <Icon name="upload" class="h-4 w-4" />
                        Import OPML
                    </span>
                </label>
                {#if data.subscriptions.length > 0}
                    <a href="/api/feeds/opml/download" class="btn w-full">
                        <Icon name="download" class="h-4 w-4" />
                        Export OPML
                    </a>
                {/if}
            </div>
        </section>

        {#if data.subscriptions.length > 0}
            <button
                onclick={clearAll}
                class="btn w-full"
                style="background:var(--danger);color:#fff"
            >
                <Icon name="trash" class="h-4 w-4" />
                Clear all subscriptions
            </button>
        {/if}
    </aside>
</div>
