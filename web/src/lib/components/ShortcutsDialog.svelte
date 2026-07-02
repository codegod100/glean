<script lang="ts">
    import Icon from "./Icon.svelte";

    interface Props {
        open: boolean;
        onClose: () => void;
    }
    let { open, onClose }: Props = $props();

    const nav = [
        ["g", "Dashboard"],
        ["a", "Articles"],
        ["f", "Feeds"],
        ["t", "Trending"],
        ["l", "Library"],
    ];
    const articles = [
        ["j", "Next article"],
        ["k", "Previous article"],
        ["o", "Open article"],
        ["m", "Toggle read"],
    ];
</script>

{#if open}
    <div
        class="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 p-4"
        onclick={onClose}
        onkeydown={(e) => e.key === "Escape" && onClose()}
        role="presentation"
    >
        <div
            class="w-full max-w-sm border-2 border-[var(--border)] bg-[var(--bg)] shadow-[6px_6px_0_0_var(--border)]"
            onclick={(e) => e.stopPropagation()}
            onkeydown={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            tabindex="-1"
        >
            <div
                class="flex items-center justify-between border-b-2 border-[var(--border)] px-5 py-3"
            >
                <h3 class="text-xs font-extrabold uppercase tracking-widest">
                    Shortcuts
                </h3>
                <button
                    onclick={onClose}
                    aria-label="Close"
                    class="hover:text-[var(--danger)]"
                    ><Icon name="x" class="h-4 w-4" /></button
                >
            </div>
            <div class="space-y-4 p-5 text-sm">
                {#each [{ title: "Navigation", rows: nav }, { title: "Articles", rows: articles }] as section}
                    <div>
                        <p
                            class="mb-2 text-[0.65rem] font-extrabold uppercase tracking-widest text-[var(--muted)]"
                        >
                            {section.title}
                        </p>
                        <div
                            class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5"
                        >
                            {#each section.rows as [k, label]}
                                <span class="kbd-brutal">{k}</span>
                                <span class="self-center text-[var(--muted)]"
                                    >{label}</span
                                >
                            {/each}
                        </div>
                    </div>
                {/each}
            </div>
        </div>
    </div>
{/if}
