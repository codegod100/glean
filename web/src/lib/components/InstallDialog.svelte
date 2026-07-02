<script lang="ts">
    import { onMount } from "svelte";
    import Icon from "./Icon.svelte";

    interface Props {
        open: boolean;
        onClose: () => void;
    }
    let { open, onClose }: Props = $props();

    interface BIP extends Event {
        prompt: () => Promise<void>;
        userChoice: Promise<{ outcome: string }>;
    }
    let deferred = $state<BIP | null>(null);

    onMount(() => {
        const h = (e: Event) => {
            e.preventDefault();
            deferred = e as BIP;
        };
        window.addEventListener("beforeinstallprompt", h);
        return () => window.removeEventListener("beforeinstallprompt", h);
    });

    const ua = $derived(
        typeof navigator !== "undefined" ? navigator.userAgent : "",
    );
    const safari = $derived(
        /iPad|iPhone|iPod/.test(ua) ||
            (typeof navigator !== "undefined" &&
                navigator.platform === "MacIntel" &&
                navigator.maxTouchPoints > 1),
    );
    const ff = $derived(/Firefox/.test(ua) && /Android/.test(ua));

    async function install() {
        if (!deferred) return;
        await deferred.prompt();
        await deferred.userChoice;
        deferred = null;
        onClose();
    }

    const steps = $derived(
        safari
            ? [
                  "Tap the Share button",
                  'Select "Add to Home Screen"',
                  'Tap "Add"',
              ]
            : ff
              ? [
                    "Open the three-dot menu",
                    'Select "Install" or "Add to Home screen"',
                    'Tap "Add"',
                ]
              : [
                    "Open the three-dot menu",
                    'Select "Add to Home screen" or "Install app"',
                    'Tap "Add"',
                ],
    );
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
                    Install
                </h3>
                <button
                    onclick={onClose}
                    aria-label="Close"
                    class="hover:text-[var(--danger)]"
                    ><Icon name="x" class="h-4 w-4" /></button
                >
            </div>
            <div class="p-5">
                {#if deferred}
                    <p class="mb-4 text-sm text-[var(--muted)]">
                        Install Glean on your device for quick access.
                    </p>
                    <button onclick={install} class="btn btn-accent w-full"
                        >Install</button
                    >
                {:else}
                    <p class="mb-3 text-sm text-[var(--muted)]">
                        To install Glean as an app:
                    </p>
                    <ol class="mb-4 space-y-2 text-sm">
                        {#each steps as s, i (i)}
                            <li class="flex gap-3">
                                <span class="font-extrabold">{i + 1}.</span
                                ><span>{s}</span>
                            </li>
                        {/each}
                    </ol>
                    <button onclick={onClose} class="btn w-full">Got it</button>
                {/if}
            </div>
        </div>
    </div>
{/if}
