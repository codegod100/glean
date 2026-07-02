<script lang="ts">
    interface Props {
        src?: string;
        size?: string;
    }
    let { src = "", size = "h-5 w-5" }: Props = $props();
    let failed = $state(false);
    $effect(() => {
        failed = false;
    });
</script>

{#if src && !failed}
    <img
        {src}
        class="{size} border border-[var(--border)] object-cover"
        loading="lazy"
        onerror={() => (failed = true)}
        alt=""
    />
{:else}
    <span
        class="inline-flex {size} items-center justify-center border border-[var(--border)] bg-[var(--surface)] text-[var(--muted)] text-[0.6rem] font-bold"
        >·</span
    >
{/if}
