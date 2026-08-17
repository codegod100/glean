<script lang="ts">
    import type { PageData } from "./$types";
    import ArticleCard from "$lib/components/ArticleCard.svelte";
    import AnnotationCard from "$lib/components/AnnotationCard.svelte";
    import EmptyState from "$lib/components/EmptyState.svelte";
    import Seo from "$lib/components/Seo.svelte";

    let { data }: { data: PageData } = $props();

    function likedHref(page: number): string {
        const p = new URLSearchParams();
        if (page > 1) p.set("liked_page", String(page));
        if (data.annot_page.page > 1)
            p.set("annot_page", String(data.annot_page.page));
        return "/library" + (p.toString() ? "?" + p.toString() : "");
    }
    function annotHref(page: number): string {
        const p = new URLSearchParams();
        if (page > 1) p.set("annot_page", String(page));
        if (data.liked_page.page > 1)
            p.set("liked_page", String(data.liked_page.page));
        return "/library" + (p.toString() ? "?" + p.toString() : "");
    }
</script>

<Seo title="Library" />

<h1 class="text-2xl font-extrabold uppercase tracking-tight mb-2">Library</h1>
<p class="mb-6 text-xs uppercase tracking-widest text-[var(--muted)]">
    Your liked articles and annotations.
</p>

<div class="grid grid-cols-1 lg:grid-cols-2 gap-6 lg:gap-12">
    <section>
        <h2
            class="mb-4 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
        >
            Liked articles
        </h2>
        <div class="space-y-3">
            {#if data.articles.length === 0}
                <EmptyState
                    icon="heart"
                    title="No liked articles yet"
                    subtitle="Like articles to save them here."
                />
            {:else}
                {#each data.articles as a (a.id)}
                    <ArticleCard article={a} navSuffix="?liked=1" />
                {/each}
            {/if}
        </div>

        {#if data.liked_page.has_prev || data.liked_page.has_next}
            <nav
                class="flex items-center justify-center gap-2 py-8 text-xs font-bold uppercase"
            >
                {#if data.liked_page.has_prev}
                    <a href={likedHref(data.liked_page.prev_page)} class="btn"
                        >&larr; Prev</a
                    >
                {:else}
                    <span class="btn opacity-30 cursor-not-allowed"
                        >&larr; Prev</span
                    >
                {/if}
                <span class="border-2 border-[var(--border)] px-3 py-2">
                    P.{data.liked_page.page}
                </span>
                {#if data.liked_page.has_next}
                    <a href={likedHref(data.liked_page.next_page)} class="btn"
                        >Next &rarr;</a
                    >
                {:else}
                    <span class="btn opacity-30 cursor-not-allowed"
                        >Next &rarr;</span
                    >
                {/if}
            </nav>
        {/if}
    </section>

    <section>
        <h2
            class="mb-4 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
        >
            Annotations
        </h2>
        <div class="space-y-3">
            {#if data.annotations.length === 0}
                <EmptyState
                    icon="note"
                    title="No annotations yet"
                    subtitle="Highlight and annotate articles as you read."
                />
            {:else}
                {#each data.annotations as a (a.id)}
                    <AnnotationCard annotation={a} userDID={data.user.did} />
                {/each}
            {/if}
        </div>

        {#if data.annot_page.has_prev || data.annot_page.has_next}
            <nav
                class="flex items-center justify-center gap-2 py-8 text-xs font-bold uppercase"
            >
                {#if data.annot_page.has_prev}
                    <a href={annotHref(data.annot_page.prev_page)} class="btn"
                        >&larr; Prev</a
                    >
                {:else}
                    <span class="btn opacity-30 cursor-not-allowed"
                        >&larr; Prev</span
                    >
                {/if}
                <span class="border-2 border-[var(--border)] px-3 py-2">
                    P.{data.annot_page.page}
                </span>
                {#if data.annot_page.has_next}
                    <a href={annotHref(data.annot_page.next_page)} class="btn"
                        >Next &rarr;</a
                    >
                {:else}
                    <span class="btn opacity-30 cursor-not-allowed"
                        >Next &rarr;</span
                    >
                {/if}
            </nav>
        {/if}
    </section>
</div>
