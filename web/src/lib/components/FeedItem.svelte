<script lang="ts">
    import type { Subscription } from "$lib/types";
    import Favicon from "./Favicon.svelte";
    import Icon from "./Icon.svelte";
    import { endpoints } from "$lib/api";
    import { invalidateAll } from "$app/navigation";

    interface Props {
        sub: Subscription;
        removable?: boolean;
    }
    let { sub, removable = true }: Props = $props();

    let editing = $state(false);
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let category = $state(sub.category ?? "");
    let removing = $state(false);

    async function save() {
        await endpoints.editFeed(sub.feed_url, category);
        editing = false;
        await invalidateAll();
    }

    async function remove() {
        removing = true;
        try {
            await endpoints.removeFeed(sub.feed_url);
            await invalidateAll();
        } finally {
            removing = false;
        }
    }
</script>

<div class="flex items-center gap-3 px-4 py-3">
    <a
        href="/articles?feed={encodeURIComponent(sub.feed_url)}"
        class="flex min-w-0 flex-1 items-center gap-3"
    >
        <Favicon src={sub.favicon_url} size="h-6 w-6" />
        <div class="min-w-0">
            <div class="flex items-center gap-2">
                <span class="truncate font-bold"
                    >{sub.feed_title || sub.feed_url}</span
                >
                {#if sub.category}<span class="tag">{sub.category}</span>{/if}
            </div>
            <div class="truncate text-[0.7rem] text-[var(--muted)]">
                {sub.feed_url}
            </div>
        </div>
    </a>
    <div class="flex shrink-0 items-center gap-2">
        {#if sub.unread_count}<span class="chip" data-active="true"
                >{sub.unread_count}</span
            >{/if}
        {#if removable}
            {#if editing}
                <form
                    onsubmit={(e) => {
                        e.preventDefault();
                        save();
                    }}
                    class="flex items-center gap-1.5"
                >
                    <input
                        bind:value={category}
                        class="input-brutal !py-1 !text-xs w-32"
                        placeholder="Category"
                    />
                    <button type="submit" class="btn !py-1 !text-[0.65rem]"
                        >Save</button
                    >
                    <button
                        type="button"
                        class="btn btn-ghost !py-1 !text-[0.65rem]"
                        onclick={() => (editing = false)}>×</button
                    >
                </form>
            {:else}
                <button
                    onclick={() => (editing = true)}
                    title="Edit"
                    class="btn btn-ghost !px-1.5 !py-1"
                    ><Icon name="edit" class="h-3.5 w-3.5" /></button
                >
                <button
                    onclick={remove}
                    disabled={removing}
                    title="Unsubscribe"
                    class="btn btn-ghost !px-1.5 !py-1 text-[var(--danger)]"
                    ><Icon name="trash" class="h-3.5 w-3.5" /></button
                >
            {/if}
        {/if}
    </div>
</div>
