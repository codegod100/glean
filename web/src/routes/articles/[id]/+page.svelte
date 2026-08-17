<script lang="ts">
    import type { PageData } from "./$types";
    import LikeButton from "$lib/components/LikeButton.svelte";
    import Favicon from "$lib/components/Favicon.svelte";
    import AnnotationCard from "$lib/components/AnnotationCard.svelte";
    import Icon from "$lib/components/Icon.svelte";
    import Seo from "$lib/components/Seo.svelte";
    import BlueskyLogo from "$lib/components/BlueskyLogo.svelte";
    import { endpoints } from "$lib/api";
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

    // Escape regex metacharacters in a literal string so it can be embedded
    // safely in a RegExp.
    function escapeRegExp(s: string): string {
        return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    }

    // Collapse runs of whitespace in the quote so it matches rendered text,
    // where HTML whitespace normalization has already happened.
    function normalizeQuote(q: string): string {
        return q.replace(/\s+/g, " ").trim();
    }

    // Strip every existing highlight under `root`, unwrapping the <mark>
    // elements so the underlying text is restored verbatim.
    function clearHighlights(root: HTMLElement) {
        for (const m of Array.from(
            root.querySelectorAll("mark.annotation-highlight"),
        )) {
            const parent = m.parentNode;
            if (!parent) continue;
            while (m.firstChild) parent.insertBefore(m.firstChild, m);
            parent.removeChild(m);
        }
        root.normalize();
    }

    // Wrap every occurrence of any annotation quote in `root` with
    // <mark class="annotation-highlight">. Handles quotes that span multiple
    // text nodes (e.g. when an inline element like <a> sits inside the quote)
    // by wrapping each affected text-node fragment in its own <mark>.
    // Idempotent when paired with clearHighlights.
    //
    // `quoteToId` maps each normalized quote to the annotation id it belongs
    // to, so cross-node fragments can still point at the right annotation card.
    function applyHighlights(
        root: HTMLElement,
        quoteToId: Map<string, number>,
    ) {
        const sorted = [...quoteToId.keys()]
            .filter((q) => q.trim().length > 0)
            .sort((a, b) => b.length - a.length);
        if (sorted.length === 0) return;
        const pattern = new RegExp(
            "(" + sorted.map(escapeRegExp).join("|") + ")",
            "gi",
        );

        const walker = document.createTreeWalker(
            root,
            NodeFilter.SHOW_TEXT,
            {
                acceptNode(node) {
                    const parent = node.parentNode as HTMLElement | null;
                    if (!parent) return NodeFilter.FILTER_REJECT;
                    const tag = parent.tagName;
                    if (
                        tag === "SCRIPT" ||
                        tag === "STYLE" ||
                        tag === "MARK" ||
                        tag === "NOSCRIPT"
                    )
                        return NodeFilter.FILTER_REJECT;
                    if (!node.nodeValue || !node.nodeValue.trim())
                        return NodeFilter.FILTER_REJECT;
                    return NodeFilter.FILTER_ACCEPT;
                },
            },
        );

        // Collect text nodes and build a flat normalized string with a map
        // back to (node, localOffset) for every character.
        const nodes: Text[] = [];
        let flat = "";
        // Each entry maps a flat-string index to the source text node and the
        // offset within that node's value. Built incrementally so we can map
        // any regex match back to its DOM position even when normalization
        // collapses whitespace runs.
        const indexMap: { node: Text; offset: number }[] = [];
        let n: Node | null;
        while ((n = walker.nextNode())) {
            const node = n as Text;
            const value = node.nodeValue ?? "";
            for (let i = 0; i < value.length; i++) {
                const ch = value[i];
                // Collapse any run of whitespace to a single space, mirroring
                // how normalizeQuote prepares the quotes.
                const isWs = /\s/.test(ch);
                const prevIsWs = flat.length > 0 && /\s/.test(flat[flat.length - 1]);
                if (isWs && prevIsWs) continue;
                flat += isWs ? " " : ch;
                indexMap.push({ node, offset: i });
            }
            nodes.push(node);
        }
        flat = flat.trim();
        if (flat.length === 0) return;

        // Find match ranges in the flat string and remember which quote each
        // match belongs to (by normalized text), so cross-node fragments can
        // tag themselves with the right annotation id.
        const matches: { start: number; end: number; quote: string }[] = [];
        let m: RegExpExecArray | null;
        pattern.lastIndex = 0;
        while ((m = pattern.exec(flat)) !== null) {
            if (m[0].length === 0) {
                pattern.lastIndex++;
                continue;
            }
            matches.push({
                start: m.index,
                end: m.index + m[0].length,
                quote: m[0],
            });
        }
        if (matches.length === 0) return;

        // Group matched text nodes so we can wrap them. For each text node,
        // compute the list of [localStart, localEnd, quote) intervals that fall
        // inside any match.
        // Build per-node intervals by walking the indexMap over match ranges.
        const byNode = new Map<
            Text,
            Array<[number, number, string]>
        >();
        for (const { start, end, quote } of matches) {
            for (let i = start; i < end; i++) {
                const entry = indexMap[i];
                if (!entry) continue;
                const list = byNode.get(entry.node) ?? [];
                const last = list[list.length - 1];
                if (last && last[1] === entry.offset && last[2] === quote)
                    last[1] = entry.offset + 1;
                else
                    list.push([entry.offset, entry.offset + 1, quote]);
                byNode.set(entry.node, list);
            }
        }
        if (byNode.size === 0) return;

        // Wrap intervals in each affected node. Process nodes in document order
        // to keep DOM mutations predictable.
        for (const node of nodes) {
            const intervals = byNode.get(node);
            if (!intervals || intervals.length === 0) continue;
            const parent = node.parentNode;
            if (!parent) continue;
            const value = node.nodeValue ?? "";

            const frag = document.createDocumentFragment();
            let cursor = 0;
            for (const [s, e, quote] of intervals) {
                if (s > cursor)
                    frag.appendChild(
                        document.createTextNode(value.slice(cursor, s)),
                    );
                const mark = document.createElement("mark");
                mark.className = "annotation-highlight";
                const id = quoteToId.get(normalizeQuote(quote));
                if (id !== undefined)
                    mark.dataset.annotationId = String(id);
                mark.textContent = value.slice(s, e);
                frag.appendChild(mark);
                cursor = e;
            }
            if (cursor < value.length)
                frag.appendChild(
                    document.createTextNode(value.slice(cursor)),
                );
            parent.replaceChild(frag, node);
        }
    }

    // Svelte action: applies highlights to the rendered body and re-applies
    // them whenever the quote list or the rendered content changes. Reads
    // the reactive values directly inside $effect so changes are tracked.
    function highlightAction(node: HTMLElement): void {
        $effect(() => {
            const quoteToId = annotationQuoteMap;
            const contentKey = showContent;
            // Touch contentKey so this effect re-runs when the article body
            // is replaced (e.g. "Fetch full content").
            void contentKey;
            // Children from {@html} are guaranteed to be present when an
            // action runs on mount; subsequent updates are also flushed
            // before effects fire.
            applyHighlights(node, quoteToId);
            // Strip our marks on cleanup so the next run starts clean and so
            // nothing leaks if the element is unmounted.
            return () => clearHighlights(node);
        });
    }

    // Map of normalized quote -> annotation id, so each highlight (including
    // cross-node fragments) can tag itself with the matching annotation.
    const annotationQuoteMap = $derived.by<Map<string, number>>(() => {
        const map = new Map<string, number>();
        for (const a of annotations) {
            if (!a.quote || !a.quote.trim()) continue;
            map.set(normalizeQuote(a.quote), a.id);
        }
        return map;
    });

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

    // Scroll to the annotation card matching a clicked highlight.
    function onBodyClick(e: MouseEvent) {
        const target = e.target as HTMLElement | null;
        const mark = target?.closest("mark.annotation-highlight");
        if (!(mark instanceof HTMLElement)) return;
        const idStr = mark.dataset.annotationId;
        if (!idStr) return;
        const card = document.getElementById(`annotation-${idStr}`);
        card?.scrollIntoView({ behavior: "smooth", block: "center" });
        card?.classList.add("ring-2", "ring-[var(--accent)]");
        setTimeout(
            () =>
                card?.classList.remove("ring-2", "ring-[var(--accent)]"),
            1600,
        );
    }

    // Tracks which article id we've already auto-marked read, so the effect
    // fires once per navigation instead of fighting toggleRead's updates.
    let markedReadId = $state<number | null>(null);

    // Mark the article read on actual navigation. The server's detail handler
    // no longer marks read, so a hover/touch preload won't mark every hovered
    // card as read; only a real visit does. Runs once per article, so
    // toggleRead stays in control of subsequent state changes.
    $effect(() => {
        const id = data.article.id;
        if (markedReadId !== id) {
            markedReadId = id;
            read = true;
            endpoints.markRead(id).catch(() => {
                if (data.article.id === id) read = false;
            });
        }
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

<Seo
    title={data.article.title}
    description={data.article.summary}
    type="article"
/>

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
            <!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
            <div
                bind:this={bodyEl}
                class="article-body"
                onclick={onBodyClick}
                use:highlightAction
            >
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
