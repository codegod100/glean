<script lang="ts">
    import type { PageData } from "./$types";
    import Favicon from "$lib/components/Favicon.svelte";
    import AnnotationCard from "$lib/components/AnnotationCard.svelte";
    import EmptyState from "$lib/components/EmptyState.svelte";
    import Icon from "$lib/components/Icon.svelte";
    import BlueskyLogo from "$lib/components/BlueskyLogo.svelte";
    import { endpoints } from "$lib/api";
    import { invalidateAll } from "$app/navigation";

    let { data }: { data: PageData } = $props();

    const isMe = $derived(data.profile_user.did === data.user.did);

    let digestSaving = $state(false);
    let expandedSaving = $state(false);

    async function toggleDigest() {
        digestSaving = true;
        try {
            await endpoints.toggleDigest(!data.digest_enabled);
            await invalidateAll();
        } finally {
            digestSaving = false;
        }
    }
    async function toggleExpanded() {
        expandedSaving = true;
        try {
            await endpoints.toggleExpandedView(!data.expanded_view);
            await invalidateAll();
        } finally {
            expandedSaving = false;
        }
    }
    async function toggleLang(code: string) {
        await endpoints.toggleLanguage(code);
        await invalidateAll();
    }
</script>

<div class="mx-auto max-w-2xl">
    <section class="panel mb-6 p-4 sm:p-6">
        <div
            class="flex flex-col sm:flex-row items-center sm:items-start gap-4 sm:gap-5 text-center sm:text-left"
        >
            {#if data.profile_user.avatar_url}
                <img
                    src={data.profile_user.avatar_url}
                    class="h-20 w-20 border-2 border-[var(--border)] object-cover"
                    alt=""
                />
            {:else}
                <div
                    class="flex h-20 w-20 items-center justify-center border-2 border-[var(--border)] bg-[var(--surface)] text-[var(--muted)]"
                >
                    <Icon name="users" class="h-10 w-10" />
                </div>
            {/if}
            <div class="min-w-0 flex-1">
                <h1
                    class="truncate text-2xl font-extrabold uppercase tracking-tight"
                >
                    {data.profile_user.display_name}
                </h1>
                <p
                    class="mt-1 flex items-center justify-center sm:justify-start gap-1.5 text-[var(--muted)]"
                >
                    <span class="font-bold">@{data.profile_user.handle}</span>
                    <a
                        href="https://bsky.app/profile/{data.profile_user
                            .handle}"
                        target="_blank"
                        rel="noopener"
                        class="text-[var(--muted)] hover:text-[var(--accent)]"
                    >
                        <BlueskyLogo class="h-4 w-4" />
                    </a>
                </p>
                <div
                    class="mt-3 flex justify-center sm:justify-start divide-x-2 divide-[var(--border)]"
                >
                    <a href="/feeds" class="px-4 first:pl-0">
                        <span class="text-xl font-extrabold"
                            >{data.subscription_count}</span
                        >
                        <span
                            class="ml-1 text-xs uppercase tracking-wide text-[var(--muted)]"
                            >feeds</span
                        >
                    </a>
                    <a href="/library" class="px-4">
                        <span class="text-xl font-extrabold"
                            >{data.annotation_count}</span
                        >
                        <span
                            class="ml-1 text-xs uppercase tracking-wide text-[var(--muted)]"
                            >annotations</span
                        >
                    </a>
                </div>
            </div>
        </div>
    </section>

    {#if isMe && data.hasLLM}
        <h2
            class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
        >
            Settings
        </h2>
        <section class="panel mb-6 divide-y-2 divide-[var(--border)]">
            <div class="flex items-center justify-between gap-4 p-5">
                <div>
                    <p class="text-sm font-bold">Daily digest</p>
                    <p class="mt-0.5 text-xs text-[var(--muted)]">
                        Show an AI-generated summary of your 50 most recent
                        unread articles on the dashboard.
                    </p>
                </div>
                <button
                    type="button"
                    onclick={toggleDigest}
                    disabled={digestSaving}
                    class="btn {data.digest_enabled ? 'btn-accent' : ''}"
                    aria-label="Toggle daily digest"
                >
                    {data.digest_enabled ? "On" : "Off"}
                </button>
            </div>
            <div class="flex items-center justify-between gap-4 p-5">
                <div>
                    <p class="text-sm font-bold">Expanded article view</p>
                    <p class="mt-0.5 text-xs text-[var(--muted)]">
                        Show full article content inline. Articles are marked as
                        read as you scroll.
                    </p>
                </div>
                <button
                    type="button"
                    onclick={toggleExpanded}
                    disabled={expandedSaving}
                    class="btn {data.expanded_view ? 'btn-accent' : ''}"
                    aria-label="Toggle expanded view"
                >
                    {data.expanded_view ? "On" : "Off"}
                </button>
            </div>
            <div class="p-5">
                <p class="text-sm font-bold">Recommendation languages</p>
                <p class="mt-0.5 mb-4 text-xs text-[var(--muted)]">
                    Filter article recommendations to specific languages. Leave
                    empty to show all.
                </p>
                <div class="flex flex-wrap gap-2">
                    {#each data.available_languages as lang}
                        <button
                            type="button"
                            onclick={() => toggleLang(lang.code)}
                            class="chip panel-press cursor-pointer"
                            data-active={data.user_languages.includes(lang.code)
                                ? "true"
                                : "false"}>{lang.name}</button
                        >
                    {/each}
                </div>
            </div>
        </section>
    {/if}

    <h2
        class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
    >
        Feeds
    </h2>
    <div class="panel mb-6 divide-y-2 divide-[var(--border)]">
        {#if data.subscriptions.length === 0}
            <EmptyState
                icon="feed"
                title="No feeds yet"
                subtitle="Subscribed feeds will appear here."
            />
        {:else}
            {#each data.subscriptions as sub (sub.id)}
                <a
                    href="/articles?feed={encodeURIComponent(sub.feed_url)}"
                    class="flex items-center gap-3 p-4 transition-colors hover:bg-[var(--bg)]"
                >
                    <Favicon src={sub.favicon_url} size="h-5 w-5" />
                    <div class="min-w-0 flex-1">
                        <div class="flex items-center gap-2 flex-wrap">
                            <span class="truncate font-bold"
                                >{sub.feed_title || sub.feed_url}</span
                            >
                            {#if sub.category}
                                <span class="tag shrink-0">{sub.category}</span>
                            {/if}
                        </div>
                        <div
                            class="mt-0.5 truncate text-xs text-[var(--muted)]"
                        >
                            {sub.feed_url}
                        </div>
                    </div>
                    <Icon
                        name="chevronRight"
                        class="h-4 w-4 shrink-0 text-[var(--muted)]"
                    />
                </a>
            {/each}
        {/if}
    </div>

    <h2
        class="mb-3 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
    >
        Recent annotations
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
</div>
