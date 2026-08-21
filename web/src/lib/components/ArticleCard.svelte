<script lang="ts">
    import type { Article } from "$lib/types";
    import Favicon from "./Favicon.svelte";
    import LikeButton from "./LikeButton.svelte";
    import Icon from "./Icon.svelte";
    import { endpoints } from "$lib/api";
    import { plainText, youtubeID } from "$lib/format";

    interface Props {
        article: Article;
        expanded?: boolean;
        navSuffix?: string;
        dismissible?: boolean;
        onDismiss?: () => void;
        linkToOriginal?: boolean;
    }
    let {
        article,
        expanded = false,
        navSuffix = "",
        dismissible = false,
        onDismiss,
        linkToOriginal = false,
    }: Props = $props();

    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let read = $state(article.is_read);
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let liked = $state(article.has_liked);
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let likeCount = $state(article.like_count);

    async function toggleRead() {
        const next = !read;
        read = next;
        try {
            if (next) await endpoints.markRead(article.id);
            else await endpoints.markUnread(article.id);
        } catch {
            read = !next;
        }
    }

    async function dismiss() {
        if (!article.url) return;
        await endpoints.dismissArticle(article.url);
        onDismiss?.();
    }

    const yt = $derived(youtubeID(article.url));
    const meta = $derived(
        [
            article.feed_title || article.feed_url,
            article.author,
            article.published ? fmt(article.published) : "",
        ]
            .filter(Boolean)
            .join(" / "),
    );
    const articleTitle = $derived(
        article.title || article.feed_title || article.feed_url,
    );

    function fmt(t: string): string {
        const d = new Date(t);
        return isNaN(d.getTime())
            ? ""
            : d.toLocaleDateString("en-US", { month: "short", day: "2-digit" });
    }
</script>

<article
    class="panel panel-press relative {expanded ? '' : ''} {!read
        ? 'border-l-[6px] border-l-[var(--accent)]'
        : ''}"
    data-article-id={article.id}
>
    <div class="p-4">
        <div class="flex items-start justify-between gap-4">
            <div class="min-w-0 flex-1">
                <a
                    href={linkToOriginal
                        ? article.url
                        : `/articles/${article.id}${navSuffix}`}
                    target={linkToOriginal ? "_blank" : undefined}
                    rel={linkToOriginal ? "noopener noreferrer" : undefined}
                    class="block {read
                        ? 'text-[var(--muted)]'
                        : 'text-[var(--fg)]'} hover:text-[var(--accent)]"
                >
                    <span
                        class="{expanded
                            ? 'text-lg'
                            : 'text-base'} font-bold leading-snug"
                        >{articleTitle}</span
                    >
                </a>
                <div
                    class="mt-2 flex items-center gap-2 text-[0.7rem] text-[var(--muted)]"
                >
                    <Favicon
                        src={article.feed_favicon_url}
                        size="h-3.5 w-3.5"
                    />
                    <span class="truncate font-medium uppercase tracking-wide"
                        >{meta}</span
                    >
                </div>
                {#if !expanded && article.summary}
                    <p class="mt-2 line-clamp-2 text-sm text-[var(--muted)]">
                        {plainText(article.summary)}
                    </p>
                {/if}
            </div>

            <div class="flex shrink-0 flex-col items-end gap-1.5">
                <LikeButton
                    articleId={article.id}
                    bind:liked
                    bind:count={likeCount}
                />
                <button
                    onclick={toggleRead}
                    title={read ? "Mark unread" : "Mark read"}
                    class="chip"
                >
                    <Icon name="check" class="h-3 w-3" />
                    <span>{read ? "Read" : "New"}</span>
                </button>
                {#if dismissible}
                    <button
                        onclick={dismiss}
                        title="Hide"
                        class="chip hover:text-[var(--danger)]"
                    >
                        <Icon name="x" class="h-3 w-3" />
                    </button>
                {/if}
            </div>
        </div>
    </div>

    {#if expanded}
        {#if yt}
            <div class="border-y-2 border-[var(--border)]">
                <div class="aspect-video w-full">
                    <iframe
                        class="h-full w-full"
                        src="https://www.youtube.com/embed/{yt}"
                        title="YouTube"
                        frameborder="0"
                        allowfullscreen
                    ></iframe>
                </div>
            </div>
        {/if}
        {#if article.content || article.full_content || article.summary}
            <div class="article-body p-4 pt-2">
                <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                {@html article.content ||
                    article.full_content ||
                    article.summary}
            </div>
        {:else}
            <div class="p-4 pt-0">
                <a
                    href={linkToOriginal
                        ? article.url
                        : `/articles/${article.id}${navSuffix}`}
                    target={linkToOriginal ? "_blank" : undefined}
                    rel={linkToOriginal ? "noopener noreferrer" : undefined}
                    class="btn">Read full &rarr;</a
                >
            </div>
        {/if}
    {/if}
</article>
