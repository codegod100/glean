<script lang="ts">
    import type { Annotation } from "$lib/types";
    import { endpoints } from "$lib/api";
    import Icon from "./Icon.svelte";
    import { invalidateAll } from "$app/navigation";

    interface Props {
        annotation: Annotation;
        userDID: string;
    }
    let { annotation, userDID }: Props = $props();

    let editing = $state(false);
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let quote = $state(annotation.quote ?? "");
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let note = $state(annotation.note ?? "");
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let tags = $state((annotation.tags ?? []).join(", "));
    let saving = $state(false);

    const mine = $derived(annotation.author_did === userDID);
    const tagsList = $derived(annotation.tags ?? []);

    async function saveEdit() {
        saving = true;
        try {
            await endpoints.deleteAnnotation(annotation.id);
            await endpoints.createAnnotation({
                feed_url: annotation.feed_url,
                article_url: annotation.article_url,
                quote,
                note,
                tags,
            });
            editing = false;
            await invalidateAll();
        } finally {
            saving = false;
        }
    }

    function cancelEdit() {
        editing = false;
        quote = annotation.quote ?? "";
        note = annotation.note ?? "";
        tags = (annotation.tags ?? []).join(", ");
    }

    async function del() {
        await endpoints.deleteAnnotation(annotation.id);
        await invalidateAll();
    }

    function when(): string {
        if (!annotation.created_at) return "";
        const d = new Date(annotation.created_at);
        return isNaN(d.getTime())
            ? ""
            : d.toLocaleDateString("en-US", { month: "short", day: "2-digit" });
    }
</script>

<div id="annotation-{annotation.id}" class="panel p-4">
    {#if editing}
        <form
            onsubmit={(e) => {
                e.preventDefault();
                saveEdit();
            }}
            class="space-y-3"
        >
            <input
                bind:value={quote}
                class="input-brutal"
                placeholder="Quote"
            />
            <textarea
                bind:value={note}
                rows="3"
                class="input-brutal resize-none"
                placeholder="Note"></textarea>
            <input
                bind:value={tags}
                class="input-brutal"
                placeholder="Tags (comma separated)"
            />
            <div class="flex justify-end gap-2">
                <button type="button" class="btn" onclick={cancelEdit}
                    >Cancel</button
                >
                <button type="submit" class="btn btn-accent" disabled={saving}
                    >Save</button
                >
            </div>
        </form>
    {:else}
        {#if annotation.article_url}
            <div
                class="mb-3 flex items-center gap-2 text-[0.7rem] text-[var(--muted)]"
            >
                {#if annotation.article_id}
                    <a
                        href="/articles/{annotation.article_id}"
                        class="inline-flex items-center gap-1 hover:text-[var(--accent)]"
                        ><Icon name="note" class="h-3 w-3" /></a
                    >
                {/if}
                <a
                    href={annotation.article_url}
                    target="_blank"
                    rel="noopener"
                    class="truncate hover:text-[var(--accent)]"
                    >{annotation.article_url}</a
                >
            </div>
        {/if}
        {#if annotation.quote}
            <blockquote class="border-l-4 border-[var(--accent)] pl-3 text-sm">
                {annotation.quote}
            </blockquote>
        {/if}
        {#if annotation.note}
            <p class="mt-3 text-sm font-medium">{annotation.note}</p>
        {/if}
        {#if tagsList.length}
            <div class="mt-3 flex flex-wrap gap-1.5">
                {#each tagsList as t}<span class="tag">{t}</span>{/each}
            </div>
        {/if}
        {#if annotation.rating}<div class="mt-2 text-sm text-[var(--accent)]">
                {"★".repeat(annotation.rating)}
            </div>{/if}
        <div
            class="mt-3 flex items-center justify-between border-t-2 border-[var(--border)] pt-3 text-[0.7rem] text-[var(--muted)]"
        >
            <div class="flex items-center gap-2">
                <a
                    href="/profile/{annotation.author_did}"
                    class="font-bold uppercase hover:text-[var(--accent)]"
                    >@{annotation.author_handle ||
                        annotation.author_did.slice(0, 12)}</a
                >
                {#if when()}<span>·</span><span>{when()}</span>{/if}
            </div>
            {#if mine}
                <div class="flex gap-3">
                    <button
                        class="font-bold uppercase hover:text-[var(--fg)]"
                        onclick={() => (editing = true)}>Edit</button
                    >
                    <button
                        class="font-bold uppercase text-[var(--danger)]"
                        onclick={del}>Delete</button
                    >
                </div>
            {/if}
        </div>
    {/if}
</div>
