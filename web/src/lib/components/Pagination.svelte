<script lang="ts">
    import type { Pagination } from "$lib/types";

    interface Props {
        page: Pagination;
        base: string;
        params?: Record<string, string>;
    }
    let { page, base, params = {} }: Props = $props();

    function href(n: number): string {
        const u = new URL(base, "http://x");
        for (const [k, v] of Object.entries(params)) {
            if (v) u.searchParams.set(k, v);
        }
        if (n > 1) u.searchParams.set("page", String(n));
        else u.searchParams.delete("page");
        return `${u.pathname}${u.search}`;
    }
</script>

{#if page.has_prev || page.has_next}
    <nav
        class="flex items-center justify-center gap-2 py-8 text-xs font-bold uppercase"
    >
        {#if page.has_prev}
            <a href={href(page.prev_page)} class="btn">&larr; Prev</a>
        {:else}
            <span class="btn opacity-30 cursor-not-allowed">&larr; Prev</span>
        {/if}
        <span class="border-2 border-[var(--border)] px-3 py-2"
            >P.{page.page}</span
        >
        {#if page.has_next}
            <a href={href(page.next_page)} class="btn">Next &rarr;</a>
        {:else}
            <span class="btn opacity-30 cursor-not-allowed">Next &rarr;</span>
        {/if}
    </nav>
{/if}
