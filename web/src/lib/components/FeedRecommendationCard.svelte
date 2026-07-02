<script lang="ts">
    import type { FeedRecommendation } from "$lib/types";
    import Favicon from "./Favicon.svelte";
    import Icon from "./Icon.svelte";
    import { endpoints } from "$lib/api";
    import { invalidateAll } from "$app/navigation";

    interface Props {
        rec: FeedRecommendation;
        onDismiss?: (feed_url: string) => void;
    }
    let { rec, onDismiss }: Props = $props();

    let subscribing = $state(false);
    let subscribed = $state(false);

    async function subscribe() {
        subscribing = true;
        try {
            await endpoints.addFeed(rec.feed_url);
            subscribed = true;
            await invalidateAll();
        } finally {
            subscribing = false;
        }
    }

    async function dismiss() {
        await endpoints.dismissFeed(rec.feed_url);
        onDismiss?.(rec.feed_url);
    }
</script>

<div class="panel p-3.5">
    <div class="flex items-center justify-between gap-2">
        <a
            href="/articles?feed={encodeURIComponent(rec.feed_url)}"
            class="flex min-w-0 flex-1 items-center gap-2.5"
        >
            <Favicon src={rec.favicon_url} size="h-5 w-5" />
            <div class="min-w-0">
                <div class="truncate text-sm font-bold">
                    {rec.title || rec.feed_url}
                </div>
                {#if rec.description}<p
                        class="truncate text-[0.7rem] text-[var(--muted)]"
                    >
                        {rec.description}
                    </p>{/if}
            </div>
        </a>
        <div class="flex shrink-0 items-center gap-2">
            <span class="text-[0.65rem] font-bold uppercase text-[var(--muted)]"
                >{rec.subscriber_count}</span
            >
            <button
                onclick={dismiss}
                title="Not interested"
                class="btn btn-ghost !px-1.5 !py-1 text-[var(--danger)]"
                ><Icon name="x" class="h-3.5 w-3.5" /></button
            >
            {#if subscribed}
                <span class="chip" data-active="true">✓</span>
            {:else}
                <button
                    onclick={subscribe}
                    disabled={subscribing}
                    class="btn btn-accent !py-1 !text-[0.65rem]"
                    >Subscribe</button
                >
            {/if}
        </div>
    </div>
</div>
