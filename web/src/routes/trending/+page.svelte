<script lang="ts">
    import type { PageData } from "./$types";
    import TrendingCard from "$lib/components/TrendingCard.svelte";
    import Pagination from "$lib/components/Pagination.svelte";
    import EmptyState from "$lib/components/EmptyState.svelte";
    import Seo from "$lib/components/Seo.svelte";

    let { data }: { data: PageData } = $props();

    const scope = $derived(data.scope);
</script>

<Seo
    title="Trending"
    description="What the Glean community is reading right now."
/>

{#if !data.user}
    <div
        class="panel mb-6 flex items-center justify-between gap-4 p-4 flex-wrap"
    >
        <p class="text-sm font-bold uppercase tracking-wide">
            Sign in to like, annotate, and get personalized recommendations.
        </p>
        <a href="/auth/login" class="btn btn-accent shrink-0">Sign in</a>
    </div>
{/if}

<div class="flex items-center justify-between mb-2 flex-wrap gap-2">
    <h1 class="text-2xl font-extrabold uppercase tracking-tight">Trending</h1>
    <div class="flex gap-2">
        <a
            href="/trending?scope=all"
            class="chip panel-press"
            data-active={scope !== "for-me" ? "true" : "false"}>All</a
        >
        <a
            href="/trending?scope=for-me"
            class="chip panel-press"
            data-active={scope === "for-me" ? "true" : "false"}>For me</a
        >
    </div>
</div>
<p class="mb-6 text-xs uppercase tracking-widest text-[var(--muted)]">
    {scope === "for-me"
        ? "Trending articles from the past 7 days, from readers with similar tastes."
        : "Most liked and discussed articles from the past 7 days across Glean."}
</p>

<div class="space-y-3">
    {#if data.trending.length === 0}
        <EmptyState
            icon="trending"
            title="No trending articles yet"
            subtitle="Start liking and annotating to see trends."
        />
    {:else}
        {#each data.trending as t (t.article_id)}
            <TrendingCard item={t} />
        {/each}
    {/if}
</div>

<Pagination page={data.pagination} base="/trending" params={{ scope }} />
