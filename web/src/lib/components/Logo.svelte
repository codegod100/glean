<script lang="ts">
    interface Props {
        size?: "sm" | "md" | "lg";
    }
    let { size = "md" }: Props = $props();
    const dims = $derived(
        size === "lg" ? "h-10 w-10" : size === "sm" ? "h-6 w-6" : "h-7 w-7",
    );
    const text = $derived(
        size === "lg" ? "text-2xl" : size === "sm" ? "text-base" : "text-lg",
    );
</script>

<a href="/" class="inline-flex items-center gap-2" aria-label="Glean home">
    <img src="/favicon.svg" class={dims} alt="Glean" />
    <span class="{text} font-extrabold tracking-tight uppercase">Glean</span>
</a>
