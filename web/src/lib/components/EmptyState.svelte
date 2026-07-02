<script lang="ts">
    import Icon from "./Icon.svelte";

    interface Props {
        icon?: string;
        title: string;
        subtitle?: string;
    }
    let { icon = "feed", title, subtitle = "" }: Props = $props();
</script>

<div class="border-2 border-[var(--border)] p-10 text-center">
    <div
        class="mx-auto mb-4 inline-flex h-14 w-14 items-center justify-center border-2 border-[var(--border)] bg-[var(--surface)]"
    >
        <Icon name={icon} class="h-7 w-7" />
    </div>
    <p class="text-sm font-bold uppercase tracking-wide">{title}</p>
    {#if subtitle}<p class="mt-1 text-xs text-[var(--muted)]">
            {subtitle}
        </p>{/if}
</div>
