<script lang="ts">
    import type { PageData } from "./$types";
    import EmptyState from "$lib/components/EmptyState.svelte";
    import Seo from "$lib/components/Seo.svelte";

    let { data }: { data: PageData } = $props();

    const categories = $derived(Object.keys(data.metrics));
</script>

<Seo title="Stats" />

<h1 class="text-2xl font-extrabold uppercase tracking-tight mb-2">Stats</h1>
<p class="mb-6 text-xs uppercase tracking-widest text-[var(--muted)]">
    Application metrics and performance data.
</p>

<div class="space-y-3">
    {#if categories.length === 0}
        <EmptyState
            icon="chart"
            title="No metrics available"
            subtitle="Metrics will appear here once the application has been running for a while."
        />
    {:else}
        {#each categories as cat}
            <section class="panel divide-y-2 divide-[var(--border)]">
                <header class="px-5 py-3 border-b-2 border-[var(--border)]">
                    <h2
                        class="text-xs font-extrabold uppercase tracking-widest"
                    >
                        {cat}
                    </h2>
                </header>
                <div>
                    {#each data.metrics[cat] as m}
                        <div
                            class="flex flex-col sm:flex-row sm:items-start justify-between gap-1 sm:gap-0 px-4 sm:px-5 py-3.5"
                        >
                            <div class="min-w-0 flex-1">
                                <div class="flex items-center gap-2 flex-wrap">
                                    <span class="text-sm font-bold"
                                        >{m.name}</span
                                    >
                                    {#if m.description}
                                        <span
                                            class="text-xs text-[var(--muted)]"
                                            >{m.description}</span
                                        >
                                    {/if}
                                </div>
                                {#if m.labels}
                                    <div class="mt-1.5 flex flex-wrap gap-1.5">
                                        {#each Object.entries(m.labels) as [k, v]}
                                            <span class="tag">{k}={v}</span>
                                        {/each}
                                    </div>
                                {/if}
                            </div>
                            <div class="shrink-0 sm:ml-4 sm:text-right">
                                {#if m.type === "gauge"}
                                    <span
                                        class="font-bold tabular-nums"
                                        style="color:var(--accent)"
                                        >{m.value.toFixed(2)}</span
                                    >
                                {:else if m.type === "counter"}
                                    <span class="font-bold tabular-nums"
                                        >{Math.round(m.value)}</span
                                    >
                                {:else}
                                    <span class="font-bold tabular-nums"
                                        >{m.value.toFixed(2)}</span
                                    >
                                {/if}
                            </div>
                        </div>
                    {/each}
                </div>
            </section>
        {/each}
    {/if}
</div>
