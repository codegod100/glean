<script lang="ts">
    import type { PersonRecommendation } from "$lib/types";
    import { endpoints } from "$lib/api";

    interface Props {
        person: PersonRecommendation;
        onDismiss?: (did: string) => void;
    }
    let { person, onDismiss }: Props = $props();

    async function dismiss(e: MouseEvent) {
        e.preventDefault();
        e.stopPropagation();
        await endpoints.dismissPerson(person.did);
        onDismiss?.(person.did);
    }
</script>

<a
    href="/profile/{person.did}"
    class="panel panel-press flex items-center gap-3 p-3"
>
    {#if person.avatar_url}
        <img
            src={person.avatar_url}
            class="h-10 w-10 border-2 border-[var(--border)] object-cover"
            alt=""
        />
    {:else}
        <span
            class="inline-flex h-10 w-10 items-center justify-center border-2 border-[var(--border)] bg-[var(--surface)] font-extrabold uppercase"
            >{(person.handle || "?").charAt(0)}</span
        >
    {/if}
    <div class="min-w-0 flex-1">
        <div class="flex items-center gap-1.5">
            <span class="truncate font-bold">@{person.handle}</span>
            {#if person.is_followed}<span class="tag">Following</span>{/if}
        </div>
        {#if person.display_name}<div
                class="truncate text-xs text-[var(--muted)]"
            >
                {person.display_name}
            </div>{/if}
    </div>
    <span class="text-[0.65rem] font-bold uppercase text-[var(--muted)]"
        >{person.common_feeds} shared</span
    >
    {#if !person.is_followed}
        <button
            onclick={dismiss}
            class="text-[var(--muted)] hover:text-[var(--danger)]"
            title="Hide"
            aria-label="Hide"
        >
            <svg
                class="h-4 w-4"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"><path d="M6 6l12 12M18 6L6 18" /></svg
            >
        </button>
    {/if}
</a>
