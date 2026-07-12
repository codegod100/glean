<script lang="ts">
    import type { PageData } from "./$types";
    import LikeButton from "$lib/components/LikeButton.svelte";
    import Favicon from "$lib/components/Favicon.svelte";
    import AnnotationCard from "$lib/components/AnnotationCard.svelte";
    import Icon from "$lib/components/Icon.svelte";
    import BlueskyLogo from "$lib/components/BlueskyLogo.svelte";
    import { endpoints } from "$lib/api";
    import { invalidateAll } from "$app/navigation";
    import { youtubeID, isEmbedURL } from "$lib/format";
    import type { Annotation } from "$lib/types";

    let { data }: { data: PageData } = $props();

    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let liked = $state(data.article.has_liked);
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let likeCount = $state(data.article.like_count);
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let read = $state(data.article.is_read);
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let annotations = $state<Annotation[]>(data.annotations);
    {
        /* svelte-ignore state_referenced_locally -- optimistic local copy */
    }
    let fullContent = $state(data.article.full_content);
    let fetchingContent = $state(false);
    let contentError = $state("");

    let popoverOpen = $state(false);
    let popoverTop = $state("0px");
    let popoverLeft = $state("0px");
    let quoteValue = $state("");
    let noteValue = $state("");
    let tagsValue = $state("");
    let submitting = $state(false);

    let bodyEl = $state<HTMLElement | null>(null);
    let popoverEl = $state<HTMLElement | null>(null);

    function goBack() {
        history.back();
    }

    const yt = $derived(youtubeID(data.article.url));
    const showContent = $derived(
        fullContent || data.article.content || data.article.summary,
    );

    $effect(() => {
        annotations = data.annotations;
    });

    function clamp(v: number, min: number, max: number) {
        return Math.max(min, Math.min(max, v));
    }

    function openForSelection() {
        const sel = window.getSelection();
        const text = sel?.toString().trim() ?? "";
        if (!text || !bodyEl) return;
        const range = sel!.getRangeAt(0);
        const rect = range.getBoundingClientRect();
        let q = text;
        if (q.length > 1000) q = q.substring(0, 1000);
        quoteValue = q;
        popoverOpen = true;
        const margin = 8;
        const width = 352;
        let left = clamp(
            rect.left + (rect.width - width) / 2,
            margin,
            window.innerWidth - width - margin,
        );
        let top = rect.bottom + margin;
        if (
            window.innerHeight - rect.bottom < 320 + margin &&
            rect.top > 320 + margin
        ) {
            top = rect.top - 320 - margin;
        }
        top = Math.max(margin, top);
        popoverLeft = left + "px";
        popoverTop = top + "px";
    }

    function onMouseUp(e: MouseEvent) {
        if (popoverEl?.contains(e.target as Node)) return;
        if (!bodyEl?.contains(e.target as Node)) {
            closePopover();
            return;
        }
        const sel = window.getSelection();
        const text = sel?.toString().trim();
        if (text) openForSelection();
    }

    // Mobile browsers (e.g. Chrome on Android) finalize text selections via
    // the selection handles and do not reliably emit mouseup afterwards. Poll
    // selectionchange so the popover appears once a selection inside the
    // article body becomes available.
    let selectionTimer: ReturnType<typeof setTimeout> | undefined;
    function onSelectionChange() {
        if (popoverEl?.contains(document.activeElement)) return;
        clearTimeout(selectionTimer);
        selectionTimer = setTimeout(() => {
            const sel = window.getSelection();
            if (!sel || sel.rangeCount === 0) return;
            const text = sel.toString().trim();
            if (!text) return;
            const range = sel.getRangeAt(0);
            if (!bodyEl?.contains(range.commonAncestorContainer)) return;
            openForSelection();
        }, 200);
    }

    function closePopover() {
        popoverOpen = false;
        quoteValue = "";
        noteValue = "";
        const sel = window.getSelection();
        sel?.removeAllRanges();
    }

    function onKeydown(e: KeyboardEvent) {
        const t = e.target as HTMLElement | null;
        if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA")) return;
        if (e.key === "Escape" && popoverOpen) closePopover();
    }

    async function submitAnnotation() {
        submitting = true;
        try {
            const res = await endpoints.createAnnotation({
                feed_url: data.article.feed_url,
                article_url: data.article.url,
                quote: quoteValue,
                note: noteValue,
                tags: tagsValue,
            });
            annotations = [...annotations, res.annotation];
            noteValue = "";
            tagsValue = "";
            quoteValue = "";
            closePopover();
        } finally {
            submitting = false;
        }
    }

    let commentNote = $state("");
    let commentTags = $state("");

    async function submitComment() {
        submitting = true;
        try {
            const res = await endpoints.createAnnotation({
                feed_url: data.article.feed_url,
                article_url: data.article.url,
                note: commentNote,
                tags: commentTags,
            });
            annotations = [...annotations, res.annotation];
            commentNote = "";
            commentTags = "";
        } finally {
            submitting = false;
        }
    }

    async function toggleRead() {
        const next = !read;
        read = next;
        try {
            if (next) await endpoints.markRead(data.article.id);
            else await endpoints.markUnread(data.article.id);
            await invalidateAll();
        } catch {
            read = !next;
        }
    }

    async function fetchContent() {
        fetchingContent = true;
        contentError = "";
        try {
            const res = await endpoints.fetchContent(data.article.id);
            fullContent = res.full_content;
        } catch (e) {
            contentError =
                e instanceof Error ? e.message : "Failed to fetch content";
        } finally {
            fetchingContent = false;
        }
    }

    function fmt(t: string | null): string {
        if (!t) return "";
        return new Date(t).toLocaleString("en-US", {
            month: "short",
            day: "2-digit",
            year: "numeric",
            hour: "2-digit",
            minute: "2-digit",
        });
    }
</script>

<svelte:window
    onmouseup={onMouseUp}
    onkeydown={onKeydown}
/>
<svelte:document onselectionchange={onSelectionChange} />

<div class="mx-auto max-w-3xl">
    <!-- Top nav -->
    <div
        class="mb-6 flex items-center justify-between border-b-2 border-[var(--border)] pb-4"
    >
        <button
            type="button"
            onclick={goBack}
            class="inline-flex items-center gap-1.5 text-[0.7rem] font-bold uppercase tracking-widest text-[var(--muted)] hover:text-[var(--fg)]"
        >
            <Icon name="arrowLeft" class="h-4 w-4" />Back
        </button>
        {#if data.next_id}
            <a
                href="/articles/{data.next_id}{data.next_suffix}"
                class="inline-flex items-center gap-1.5 text-[0.7rem] font-bold uppercase tracking-widest text-[var(--muted)] hover:text-[var(--fg)]"
            >
                Next<Icon name="arrowRight" class="h-4 w-4" />
            </a>
        {/if}
    </div>

    <article>
        <h1
            class="text-2xl font-extrabold uppercase leading-tight tracking-tight md:text-3xl"
        >
            {#if data.article.url}
                <a
                    href={data.article.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="hover:text-[var(--accent)]">{data.article.title}</a
                >
            {:else}
                {data.article.title}
            {/if}
        </h1>

        <!-- Meta -->
        <div
            class="mt-4 flex flex-wrap items-center gap-2.5 text-xs text-[var(--muted)]"
        >
            {#if data.article.author}<span class="font-bold text-[var(--fg)]"
                    >{data.article.author}</span
                >{/if}
            {#if data.article.published}<span>·</span><span
                    >{fmt(data.article.published)}</span
                >{/if}
            {#if data.feed?.feed_url}
                <span>·</span>
                <a
                    href="/articles?feed={encodeURIComponent(
                        data.feed.feed_url,
                    )}"
                    class="inline-flex items-center gap-1.5 hover:text-[var(--accent)]"
                >
                    <Favicon src={data.feed.favicon_url} size="h-4 w-4" />
                    {data.feed.title || data.feed.feed_url}
                </a>
            {/if}
        </div>

        <!-- Action bar -->
        <div class="mt-6 flex flex-wrap items-center gap-2">
            <LikeButton
                articleId={data.article.id}
                bind:liked
                bind:count={likeCount}
            />
            <button
                onclick={toggleRead}
                title={read ? "Mark as unread" : "Mark as read"}
                class="chip"
                data-active={read}
            >
                <Icon name="check" class="h-3.5 w-3.5" />{read
                    ? "Unread"
                    : "Read"}
            </button>
            {#if data.article.url}
                <a
                    href={data.article.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    title="Open original"
                    class="chip"
                >
                    <Icon name="external" class="h-3.5 w-3.5" />Original
                </a>
                <a
                    href="https://bsky.app/intent/compose?text={encodeURIComponent(
                        data.article.title + ' ' + data.article.url,
                    )}"
                    target="_blank"
                    rel="noopener noreferrer"
                    title="Share on Bluesky"
                    class="chip"
                >
                    <BlueskyLogo class="h-3.5 w-3.5" />Share
                </a>
            {/if}
        </div>

        {#if yt}
            <div
                class="my-8 aspect-video w-full overflow-hidden border-2 border-[var(--border)] shadow-[4px_4px_0_0_var(--border)]"
            >
                <iframe
                    class="h-full w-full"
                    src="https://www.youtube.com/embed/{yt}"
                    title="YouTube video player"
                    frameborder="0"
                    allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
                    referrerpolicy="strict-origin-when-cross-origin"
                    allowfullscreen
                ></iframe>
            </div>
        {/if}

        {#if showContent}<hr
                class="my-8 border-t-2 border-[var(--border)]"
            />{/if}

        {#if showContent}
            <div bind:this={bodyEl} class="article-body">
                <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                {@html showContent}
            </div>
        {/if}

        {#if !data.article.content && !fullContent && data.article.url && !isEmbedURL(data.article.url)}
            <div class="mt-6">
                {#if fetchingContent}
                    <span class="text-sm text-[var(--muted)]">Fetching…</span>
                {:else if contentError}
                    <div class="text-sm text-[var(--muted)]">
                        Failed to fetch content. <button
                            class="text-[var(--accent)] underline"
                            onclick={fetchContent}>Retry</button
                        >
                    </div>
                {:else}
                    <button onclick={fetchContent} class="btn">
                        <Icon
                            name="refresh"
                            class="h-3.5 w-3.5"
                            strokeWidth={1.5}
                        />Fetch full content
                    </button>
                {/if}
            </div>
        {:else if !data.article.url}
            <p class="text-[var(--muted)]">No content available.</p>
        {/if}
    </article>

    <hr class="my-8 border-t-2 border-[var(--border)]" />

    <!-- Annotations -->
    <section>
        <h2
            class="mb-5 text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
        >
            Annotations
        </h2>

        <form
            onsubmit={(e) => {
                e.preventDefault();
                submitComment();
            }}
            class="panel mb-4 space-y-3 p-4"
        >
            <textarea
                bind:value={commentNote}
                rows="2"
                placeholder="Add a comment..."
                class="input-brutal resize-none"></textarea>
            <div class="flex gap-2">
                <input
                    bind:value={commentTags}
                    type="text"
                    placeholder="Tags (comma separated)"
                    class="input-brutal min-w-0 flex-1"
                />
                <button
                    type="submit"
                    disabled={submitting}
                    class="btn btn-accent shrink-0">Comment</button
                >
            </div>
        </form>

        <div class="space-y-3">
            {#if annotations.length === 0}
                <p class="py-6 text-center text-sm text-[var(--muted)]">
                    No annotations yet. Select text above or add a note.
                </p>
            {:else}
                {#each annotations as a (a.id)}
                    <AnnotationCard
                        annotation={a}
                        userDID={data.current_user_did}
                    />
                {/each}
            {/if}
        </div>
    </section>

    <!-- Selection popover -->
    {#if popoverOpen}
        <div
            bind:this={popoverEl}
            class="panel fixed z-50 w-[22rem] max-w-[calc(100vw-2rem)] space-y-3 p-4 shadow-[6px_6px_0_0_var(--border)]"
            style="left: {popoverLeft}; top: {popoverTop};"
        >
            <div class="flex items-center justify-between">
                <span
                    class="text-[0.65rem] font-extrabold uppercase tracking-widest text-[var(--muted)]"
                    >Annotate</span
                >
                <button
                    type="button"
                    onclick={closePopover}
                    aria-label="Close"
                    class="text-[var(--muted)] hover:text-[var(--fg)]"
                >
                    <Icon name="x" class="h-4 w-4" />
                </button>
            </div>
            {#if quoteValue}
                <blockquote
                    class="border-l-[6px] border-l-[var(--accent)] pl-3 text-sm italic text-[var(--muted)]"
                >
                    {quoteValue}
                </blockquote>
            {/if}
            <textarea
                bind:value={noteValue}
                rows="2"
                placeholder="Add a note..."
                class="input-brutal resize-none"></textarea>
            <div class="flex gap-2">
                <input
                    bind:value={tagsValue}
                    type="text"
                    placeholder="Tags (comma separated)"
                    class="input-brutal min-w-0 flex-1"
                />
                <button
                    type="button"
                    onclick={submitAnnotation}
                    disabled={submitting}
                    class="btn btn-accent shrink-0">Annotate</button
                >
            </div>
        </div>
    {/if}

    <!-- Bottom nav -->
    <div
        class="mt-8 mb-4 flex items-center justify-between border-t-2 border-[var(--border)] pt-4"
    >
        <button
            type="button"
            onclick={goBack}
            class="inline-flex items-center gap-1.5 text-[0.7rem] font-bold uppercase tracking-widest text-[var(--muted)] hover:text-[var(--fg)]"
        >
            <Icon name="arrowLeft" class="h-4 w-4" />Back
        </button>
        {#if data.next_id}
            <a
                href="/articles/{data.next_id}{data.next_suffix}"
                class="inline-flex items-center gap-1.5 text-[0.7rem] font-bold uppercase tracking-widest text-[var(--muted)] hover:text-[var(--fg)]"
            >
                Next<Icon name="arrowRight" class="h-4 w-4" />
            </a>
        {/if}
    </div>
</div>
