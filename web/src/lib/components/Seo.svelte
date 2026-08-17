<script lang="ts">
    import { page } from "$app/state";

    interface Props {
        title?: string;
        description?: string;
        type?: string;
        noindex?: boolean;
    }

    let {
        title,
        description,
        type = "website",
        noindex = false,
    }: Props = $props();

    const siteName = "Glean";
    const pageTitle = $derived(
        title ? `${title} — ${siteName}` : `${siteName} — The social RSS reader`,
    );
    const canonical = $derived(page.url.origin + page.url.pathname);
    const absoluteImage = $derived(new URL("/banner.png", page.url).href);
    const plainDescription = $derived(
        (description ?? "")
            .replace(/<[^>]*>/g, " ")
            .replace(/\s+/g, " ")
            .trim()
            .slice(0, 200),
    );
</script>

<svelte:head>
    <title>{pageTitle}</title>
    {#if plainDescription}
        <meta name="description" content={plainDescription} />
    {/if}
    <link rel="canonical" href={canonical} />
    {#if noindex}
        <meta name="robots" content="noindex" />
    {/if}
    <meta property="og:site_name" content={siteName} />
    <meta property="og:title" content={pageTitle} />
    {#if plainDescription}
        <meta property="og:description" content={plainDescription} />
    {/if}
    <meta property="og:type" content={type} />
    <meta property="og:url" content={canonical} />
    <meta property="og:image" content={absoluteImage} />
    <meta name="twitter:card" content="summary_large_image" />
    <meta name="twitter:title" content={pageTitle} />
    {#if plainDescription}
        <meta name="twitter:description" content={plainDescription} />
    {/if}
    <meta name="twitter:image" content={absoluteImage} />
</svelte:head>
