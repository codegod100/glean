<script lang="ts">
    import { onMount } from "svelte";
    import { invalidateAll } from "$app/navigation";
    import { endpoints } from "$lib/api";
    import Icon from "./Icon.svelte";

    interface Props {
        /** Unix seconds serving as the "last seen" baseline (e.g. page load time). */
        since: number;
        /** Polling interval in milliseconds. */
        interval?: number;
    }
    let { since, interval = 60_000 }: Props = $props();

    let count = $state(0);
    let timer: ReturnType<typeof setInterval>;

    onMount(() => {
        timer = setInterval(poll, interval);
        return () => clearInterval(timer);
    });

    async function poll() {
        // Hide the banner for inactive tabs; the next visibility change polls.
        if (document.hidden) return;
        try {
            const res = await endpoints.newArticleCount(since);
            count = res.count;
        } catch {
            count = 0;
        }
    }

    async function refresh() {
        since = Math.floor(Date.now() / 1000);
        count = 0;
        await invalidateAll();
    }
</script>

{#if count > 0}
    <div
        class="mb-4 flex items-center justify-between gap-3 border-2 border-[var(--border)] bg-[var(--accent)] px-4 py-2.5 text-[var(--accent-ink)] shadow-[4px_4px_0_0_var(--border)] dark:text-white"
        role="status"
    >
        <span class="flex items-center gap-2 text-sm font-bold">
            <Icon name="sparkles" class="h-4 w-4" />
            {count} new article{count === 1 ? "" : "s"}
        </span>
        <button
            type="button"
            onclick={refresh}
            class="border-2 border-[var(--border)] bg-[var(--bg)] px-3 py-1 text-xs font-bold uppercase tracking-wide text-[var(--fg)] shadow-[2px_2px_0_0_var(--border)] transition-transform hover:-translate-x-px hover:-translate-y-px"
        >
            Show
        </button>
    </div>
{/if}
