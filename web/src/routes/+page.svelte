<script lang="ts">
    // Landing page is static; auth gating happens in +page.server.ts.
    import Logo from "$lib/components/Logo.svelte";
    import Icon from "$lib/components/Icon.svelte";
    import Seo from "$lib/components/Seo.svelte";
</script>

<Seo
    description="The social RSS reader built on AT Protocol. Read your feeds, see what your circle reads, and discover new sources through personalized recommendations."
/>

<!-- HERO -->
<section class="border-b-2 border-[var(--border)]">
    <div
        class="mx-auto grid max-w-5xl grid-cols-1 gap-10 px-4 py-16 lg:grid-cols-2 lg:items-center lg:py-24"
    >
        <div>
            <div class="mb-8"><Logo size="lg" /></div>
            <h1
                class="text-4xl font-extrabold uppercase leading-[0.95] tracking-tight md:text-6xl"
            >
                The social<br />RSS reader.
            </h1>
            <p
                class="mt-6 max-w-md text-sm leading-relaxed text-[var(--muted)]"
            >
                Read your feeds. See what your circle reads. Discover new
                sources through personalized recommendations. All on AT
                Protocol.
            </p>
            <div class="mt-8 flex flex-col gap-3 sm:flex-row">
                <a href="/auth/login" class="btn btn-accent">Sign in</a>
                <a href="/trending" class="btn">See what's trending</a>
            </div>
        </div>

        <!-- Mock dashboard panel -->
        <div
            class="panel hidden p-0 shadow-[6px_6px_0_0_var(--border)] lg:block"
        >
            <div
                class="flex items-center gap-2 border-b-2 border-[var(--border)] px-3 py-2"
            >
                <span
                    class="h-2.5 w-2.5 border-2 border-[var(--border)] bg-[var(--accent)]"
                ></span>
                <span class="h-2.5 w-2.5 border-2 border-[var(--border)]"
                ></span>
                <span class="h-2.5 w-2.5 border-2 border-[var(--border)]"
                ></span>
                <span
                    class="ml-2 text-[0.65rem] font-bold uppercase tracking-widest text-[var(--muted)]"
                    >Glean — Dashboard</span
                >
            </div>
            <div class="divide-y-2 divide-[var(--border)]">
                <div class="flex items-start gap-3 px-4 py-3">
                    <span
                        class="inline-flex h-7 w-7 shrink-0 items-center justify-center border-2 border-[var(--border)] bg-[var(--accent)] text-xs font-extrabold text-white"
                        >★</span
                    >
                    <div class="min-w-0 flex-1">
                        <p class="text-[0.8rem] font-bold leading-tight">
                            Why the open web is making a comeback
                        </p>
                        <p
                            class="mt-0.5 text-[0.65rem] uppercase tracking-wide text-[var(--muted)]"
                        >
                            theopenweb.press · 2h
                        </p>
                    </div>
                    <span class="chip shrink-0" data-active="true">♥ 12</span>
                </div>
                <div
                    class="flex items-start gap-3 border-l-[6px] border-l-[var(--accent)] px-4 py-3"
                >
                    <span
                        class="inline-flex h-7 w-7 shrink-0 items-center justify-center border-2 border-[var(--border)] bg-[var(--surface)] text-xs"
                        >📰</span
                    >
                    <div class="min-w-0 flex-1">
                        <p class="text-[0.8rem] font-bold leading-tight">
                            How to take back control of your reading list
                        </p>
                        <p
                            class="mt-0.5 text-[0.65rem] uppercase tracking-wide text-[var(--muted)]"
                        >
                            readwrite.cafe · 5h
                        </p>
                    </div>
                </div>
                <div class="flex items-start gap-3 px-4 py-3">
                    <span
                        class="inline-flex h-7 w-7 shrink-0 items-center justify-center border-2 border-[var(--border)] bg-[var(--surface)] text-xs"
                        >🔥</span
                    >
                    <div class="min-w-0 flex-1">
                        <p class="text-[0.8rem] font-bold leading-tight">
                            RSS is not dead, it was just resting
                        </p>
                        <p
                            class="mt-0.5 text-[0.65rem] uppercase tracking-wide text-[var(--muted)]"
                        >
                            syndication.xyz · 8h
                        </p>
                    </div>
                </div>
            </div>
            <div
                class="flex items-center justify-between border-t-2 border-[var(--border)] px-4 py-2 text-[0.65rem] uppercase tracking-wide text-[var(--muted)]"
            >
                <span>4 trending in your network</span>
                <span class="font-extrabold text-[var(--accent)]"
                    >42 unread</span
                >
            </div>
        </div>
    </div>
</section>

<!-- FEATURES -->
<section class="border-b-2 border-[var(--border)]">
    <div class="mx-auto max-w-5xl px-4 py-16">
        <p
            class="text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
        >
            // Features
        </p>
        <h2
            class="mt-2 text-2xl font-extrabold uppercase tracking-tight md:text-3xl"
        >
            Read with intent
        </h2>
        <div
            class="mt-10 grid grid-cols-1 gap-0 border-2 border-[var(--border)] md:grid-cols-3"
        >
            <div
                class="border-b-2 border-[var(--border)] p-6 md:border-b-0 md:border-r-2"
            >
                <Icon name="note" class="h-6 w-6 text-[var(--accent)]" />
                <h3 class="mt-4 text-sm font-extrabold uppercase tracking-wide">
                    Highlight
                </h3>
                <p class="mt-2 text-xs leading-relaxed text-[var(--muted)]">
                    Select any passage in an article. Your highlights live in
                    your personal data repository.
                </p>
            </div>
            <div
                class="border-b-2 border-[var(--border)] p-6 md:border-b-0 md:border-r-2"
            >
                <Icon name="note" class="h-6 w-6 text-[var(--accent)]" />
                <h3 class="mt-4 text-sm font-extrabold uppercase tracking-wide">
                    Annotate
                </h3>
                <p class="mt-2 text-xs leading-relaxed text-[var(--muted)]">
                    Write notes for your future self. Tag them. Find them later
                    in your library.
                </p>
            </div>
            <div class="p-6">
                <Icon name="bookmark" class="h-6 w-6 text-[var(--accent)]" />
                <h3 class="mt-4 text-sm font-extrabold uppercase tracking-wide">
                    Save
                </h3>
                <p class="mt-2 text-xs leading-relaxed text-[var(--muted)]">
                    Keep articles around. Like them to boost them into your
                    friends' feeds.
                </p>
            </div>
        </div>
    </div>
</section>

<!-- OPEN / AT PROTOCOL -->
<section
    class="border-b-2 border-[var(--border)] bg-[var(--accent)] text-white"
>
    <div class="mx-auto max-w-5xl px-4 py-16">
        <p class="text-xs font-extrabold uppercase tracking-widest opacity-70">
            // Open
        </p>
        <h2
            class="mt-2 max-w-2xl text-2xl font-extrabold uppercase tracking-tight md:text-3xl"
        >
            Your feeds. Your data. Yours.
        </h2>
        <p class="mt-5 max-w-2xl text-sm leading-relaxed opacity-80">
            Glean is part of the Atmosphere. Annotations are compatible with
            <a
                href="https://margin.at"
                target="_blank"
                rel="noopener"
                class="underline underline-offset-2 hover:text-white"
                >Margin.at</a
            >
            notes, feeds with
            <a
                href="https://skyreader.app"
                target="_blank"
                rel="noopener"
                class="underline underline-offset-2 hover:text-white"
                >Skyreader</a
            >
            subscriptions, and your social graph travels with you from
            <a
                href="https://bsky.social"
                target="_blank"
                rel="noopener"
                class="underline underline-offset-2 hover:text-white">Bluesky</a
            >
            and
            <a
                href="https://tangled.org"
                target="_blank"
                rel="noopener"
                class="underline underline-offset-2 hover:text-white">Tangled</a
            >. All on AT Protocol.
        </p>
        <div class="mt-8 grid grid-cols-2 gap-4 md:grid-cols-4">
            <span
                class="border-2 border-white/40 px-3 py-2 text-[0.7rem] font-bold uppercase tracking-wide"
                >AT Protocol identity</span
            >
            <span
                class="border-2 border-white/40 px-3 py-2 text-[0.7rem] font-bold uppercase tracking-wide"
                >Portable data</span
            >
            <a
                href="https://tangled.org/julien.rbrt.fr/glean"
                class="border-2 border-white/40 px-3 py-2 text-[0.7rem] font-bold uppercase tracking-wide hover:bg-white hover:text-[var(--accent)]"
                >Open source</a
            >
            <a
                href="https://tangled.org/julien.rbrt.fr/glean"
                target="_blank"
                rel="noopener"
                class="border-2 border-white/40 px-3 py-2 text-[0.7rem] font-bold uppercase tracking-wide hover:bg-white hover:text-[var(--accent)]"
                >Self-hostable</a
            >
        </div>
    </div>
</section>

<!-- FINAL CTA -->
<section class="py-20 text-center">
    <div class="mx-auto max-w-md px-4">
        <Logo size="md" />
        <h2
            class="mt-6 text-2xl font-extrabold uppercase tracking-tight md:text-3xl"
        >
            Start reading
        </h2>
        <p class="mt-3 text-xs leading-relaxed text-[var(--muted)]">
            Sign in with your Bluesky handle or any Atmosphere account.
        </p>
        <a href="/auth/login" class="btn btn-accent mt-6">Get started</a>
    </div>
</section>
