<script lang="ts">
    import type { TrendingItem } from "$lib/types";
    import Favicon from "./Favicon.svelte";
    import LikeButton from "./LikeButton.svelte";
    import Icon from "./Icon.svelte";
    import { plainText } from "$lib/format";

    interface Props {
        item: TrendingItem;
        linkToOriginal?: boolean;
    }
    let { item, linkToOriginal = false }: Props = $props();
</script>

<div class="panel panel-press p-4">
    <div class="flex items-start justify-between gap-4">
        <div class="min-w-0 flex-1">
            <a
                href={linkToOriginal
                    ? item.url
                    : `/articles/${item.article_id}`}
                target={linkToOriginal ? "_blank" : undefined}
                rel={linkToOriginal ? "noopener noreferrer" : undefined}
                class="block text-base font-bold leading-snug hover:text-[var(--accent)]"
                >{item.title}</a
            >
            <div
                class="mt-2 flex items-center gap-2 text-[0.7rem] text-[var(--muted)]"
            >
                <Favicon src={item.favicon_url} size="h-3.5 w-3.5" />
                <span class="truncate font-medium uppercase tracking-wide"
                    >{item.feed_title || item.feed_url}{item.author
                        ? " / " + item.author
                        : ""}</span
                >
            </div>
            {#if item.summary}
                <p class="mt-2 line-clamp-2 text-sm text-[var(--muted)]">
                    {plainText(item.summary)}
                </p>
            {/if}
        </div>
        <div class="flex shrink-0 flex-col items-end gap-1.5">
            <LikeButton
                articleId={item.article_id}
                liked={item.has_liked}
                count={item.like_count}
            />
            <span class="chip" title="Annotations">
                <Icon name="note" class="h-3 w-3" />
                {item.annotation_count}
            </span>
        </div>
    </div>
</div>
