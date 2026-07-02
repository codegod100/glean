<script lang="ts">
    interface Props {
        block?: boolean;
    }
    let { block = false }: Props = $props();

    let theme = $state("light");
    $effect(() => {
        theme =
            (typeof document !== "undefined" &&
                document.documentElement.getAttribute("data-theme")) ||
            "light";
    });

    function toggle() {
        theme = theme === "dark" ? "light" : "dark";
        localStorage.setItem("theme", theme);
        document.documentElement.setAttribute("data-theme", theme);
    }
</script>

{#if block}
    <button
        onclick={toggle}
        class="block font-bold uppercase hover:text-[var(--accent)]"
        >{theme === "dark" ? "Light" : "Dark"} mode</button
    >
{:else}
    <button
        onclick={toggle}
        class="btn btn-ghost px-2"
        title="Toggle theme"
        aria-label="Toggle theme"
    >
        <svg
            class="h-4 w-4"
            fill="none"
            stroke="currentColor"
            stroke-width="1.75"
            viewBox="0 0 24 24"
        >
            {#if theme === "dark"}
                <path
                    d="M12 4V2M12 22v-2M4 12H2M22 12h-2M6 6L4 4M20 4l-2 2M6 18l-2 2M20 20l-2-2M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z"
                />
            {:else}
                <path d="M21 13a9 9 0 1 1-10-10 7 7 0 0 0 10 10z" />
            {/if}
        </svg>
    </button>
{/if}
