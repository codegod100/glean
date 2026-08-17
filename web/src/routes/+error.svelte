<script lang="ts">
    import { page } from "$app/state";
    import Logo from "$lib/components/Logo.svelte";
    import Seo from "$lib/components/Seo.svelte";
</script>

<Seo
    title={page.status === 404 ? "Not found" : "Error " + page.status}
    noindex
/>

<div class="flex min-h-screen flex-col items-center justify-center px-4 py-12">
    <div class="w-full max-w-sm text-center">
        <div class="mb-8 flex justify-center"><Logo size="lg" /></div>
        <div class="mb-2 text-6xl font-extrabold text-[var(--accent)]">
            {page.status}
        </div>
        {#if page.status === 404}
            <h1 class="text-xl font-extrabold uppercase tracking-wide">
                Not found
            </h1>
            <p class="mt-2 text-sm text-[var(--muted)]">
                This page doesn't exist.
            </p>
        {:else}
            <h1 class="text-xl font-extrabold uppercase tracking-wide">
                Error
            </h1>
            <p class="mt-2 text-sm text-[var(--muted)]">
                {page.error?.message ?? "Something went wrong."}
            </p>
        {/if}
        <a href="/" class="btn btn-accent mt-8">Home</a>
    </div>
</div>
