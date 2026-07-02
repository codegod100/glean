<script lang="ts">
    import "../app.css";
    import { page } from "$app/state";
    import { goto } from "$app/navigation";
    import type { LayoutData } from "./$types";
    import Logo from "$lib/components/Logo.svelte";
    import Icon from "$lib/components/Icon.svelte";
    import ThemeToggle from "$lib/components/ThemeToggle.svelte";
    import ShortcutsDialog from "$lib/components/ShortcutsDialog.svelte";
    import InstallDialog from "$lib/components/InstallDialog.svelte";
    import { setCsrfToken, endpoints } from "$lib/api";

    let {
        data,
        children,
    }: { data: LayoutData; children: import("svelte").Snippet } = $props();

    $effect(() => {
        setCsrfToken(data.csrfToken);
    });

    const chromeless = $derived(
        page.url.pathname === "/" ||
            page.url.pathname.startsWith("/auth/") ||
            page.url.pathname === "/terms",
    );

    let menuOpen = $state(false);
    let showShortcuts = $state(false);
    let showInstall = $state(false);

    const navItems = $derived(
        data.user
            ? [
                  {
                      href: "/dashboard",
                      label: "Dashboard",
                      icon: "grid",
                      match: "/dashboard",
                  },
                  {
                      href: "/articles",
                      label: "Articles",
                      icon: "feed",
                      match: "/articles",
                  },
                  {
                      href: "/trending",
                      label: "Trending",
                      icon: "trending",
                      match: "/trending",
                  },
                  {
                      href: "/feeds",
                      label: "Feeds",
                      icon: "globe",
                      match: "/feeds",
                  },
                  {
                      href: "/library",
                      label: "Library",
                      icon: "note",
                      match: "/library",
                  },
              ]
            : [
                  {
                      href: "/trending",
                      label: "Trending",
                      icon: "trending",
                      match: "/trending",
                  },
                  {
                      href: "/articles",
                      label: "Articles",
                      icon: "feed",
                      match: "/articles",
                  },
              ],
    );

    function isActive(match: string): boolean {
        const p = page.url.pathname;
        return match === "/articles"
            ? p.startsWith("/articles")
            : p === match || p.startsWith(match + "/");
    }

    async function logout() {
        try {
            await endpoints.authLogout();
        } catch {
            // ignore
        }
        menuOpen = false;
        goto("/", { invalidateAll: true });
    }

    function handleKeydown(e: KeyboardEvent) {
        const t = e.target as HTMLElement | null;
        if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA")) return;
        if (e.ctrlKey || e.metaKey || e.altKey) return;
        if (!data.user) return;
        const map: Record<string, string> = {
            g: "/dashboard",
            a: "/articles",
            f: "/feeds",
            t: "/trending",
            l: "/library",
        };
        if (map[e.key]) goto(map[e.key]);
    }
</script>

<svelte:head>
    <title>Glean</title>
    <meta property="og:title" content="Glean" />
    <meta
        property="og:description"
        content="The social RSS reader built on AT Protocol."
    />
    <meta property="og:image" content="/banner.png" />
    <meta property="og:type" content="website" />
    <meta name="theme-color" content="#00754A" />
    <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
    <link rel="manifest" href="/manifest.json" />
</svelte:head>

<svelte:window onkeydown={handleKeydown} />

<div class="min-h-screen flex flex-col">
    <!-- Top header bar -->
    <header
        class="sticky top-0 z-30 border-b-2 border-[var(--border)] bg-[var(--bg)]"
    >
        <div class="mx-auto flex h-14 max-w-5xl items-center gap-4 px-4">
            <Logo />

            {#if !chromeless}
                <nav class="hidden items-center gap-1 md:flex">
                    {#each navItems as item}
                        <a
                            href={item.href}
                            class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-bold uppercase tracking-wide transition {isActive(
                                item.match,
                            )
                                ? 'bg-[var(--fg)] text-[var(--bg)]'
                                : 'text-[var(--muted)] hover:text-[var(--fg)] hover:bg-[var(--surface)]'}"
                        >
                            <Icon name={item.icon} class="h-3.5 w-3.5" />
                            {item.label}
                        </a>
                    {/each}
                </nav>

                <div class="ml-auto flex items-center gap-2">
                    <button
                        class="btn btn-ghost px-2"
                        title="Search"
                        onclick={() => goto("/articles")}
                        aria-label="Search"
                    >
                        <Icon name="search" class="h-4 w-4" />
                    </button>
                    <button
                        class="btn btn-ghost hidden px-2 sm:inline-flex"
                        onclick={() => (showShortcuts = true)}
                        title="Shortcuts"
                        aria-label="Shortcuts"
                    >
                        <Icon name="keyboard" class="h-4 w-4" />
                    </button>
                    {#if data.user}
                        <div class="relative">
                            <button
                                class="flex items-center gap-2 border-2 border-[var(--border)] px-2 py-1"
                                onclick={() => (menuOpen = !menuOpen)}
                            >
                                {#if data.user.avatar_url}
                                    <img
                                        src={data.user.avatar_url}
                                        class="h-6 w-6 border border-[var(--border)] object-cover"
                                        alt=""
                                    />
                                {/if}
                                <span class="text-xs font-bold"
                                    >@{data.user.handle}</span
                                >
                                <Icon name="chevronDown" class="h-3 w-3" />
                            </button>
                            {#if menuOpen}
                                <button
                                    class="fixed inset-0 z-40 cursor-default"
                                    onclick={() => (menuOpen = false)}
                                    aria-label="Close menu"
                                    tabindex="-1"
                                ></button>
                                <div
                                    class="absolute right-0 top-full z-50 mt-1 w-48 border-2 border-[var(--border)] bg-[var(--bg)] shadow-[4px_4px_0_0_var(--border)]"
                                >
                                    <a
                                        href="/profile/{data.user.did}"
                                        class="block border-b-2 border-[var(--border)] px-4 py-2.5 text-xs font-bold uppercase hover:bg-[var(--surface)]"
                                        onclick={() => (menuOpen = false)}
                                        >Profile</a
                                    >
                                    <button
                                        class="block w-full px-4 py-2.5 text-left text-xs font-bold uppercase hover:bg-[var(--surface)]"
                                        onclick={() => (showInstall = true)}
                                        >Install App</button
                                    >
                                    <button
                                        class="block w-full border-t-2 border-[var(--border)] px-4 py-2.5 text-left text-xs font-bold uppercase text-[var(--danger)] hover:bg-[var(--surface)]"
                                        onclick={logout}>Sign out</button
                                    >
                                </div>
                            {/if}
                        </div>
                    {:else}
                        <a href="/auth/login" class="btn btn-accent">Sign in</a>
                    {/if}
                </div>
            {:else}
                <div class="ml-auto flex items-center gap-2">
                    <ThemeToggle />
                    {#if !data.user}
                        <a href="/auth/login" class="btn btn-accent">Sign in</a>
                    {/if}
                </div>
            {/if}
        </div>

        <!-- Mobile nav row -->
        {#if !chromeless}
            <nav
                class="flex items-center gap-1 overflow-x-auto border-t-2 border-[var(--border)] px-2 py-1 md:hidden"
            >
                {#each navItems as item}
                    <a
                        href={item.href}
                        class="inline-flex shrink-0 items-center gap-1.5 px-2.5 py-1.5 text-[0.7rem] font-bold uppercase {isActive(
                            item.match,
                        )
                            ? 'bg-[var(--fg)] text-[var(--bg)]'
                            : 'text-[var(--muted)]'}"
                    >
                        <Icon name={item.icon} class="h-3.5 w-3.5" />
                        {item.label}
                    </a>
                {/each}
            </nav>
        {/if}
    </header>

    <!-- Content -->
    <main class="mx-auto w-full max-w-5xl flex-1 px-4 py-8">
        {@render children()}
    </main>

    <!-- Footer -->
    <footer class="border-t-2 border-[var(--border)]">
        <div
            class="mx-auto flex max-w-5xl flex-col gap-6 px-4 py-8 text-xs md:flex-row md:items-start md:justify-between"
        >
            <div class="space-y-2">
                <Logo size="sm" />
                <p class="max-w-xs text-[var(--muted)]">
                    The social RSS reader on AT Protocol.
                </p>
            </div>
            <div class="flex flex-wrap gap-x-10 gap-y-6">
                <div class="space-y-1.5">
                    <p
                        class="text-[0.65rem] font-extrabold uppercase tracking-widest text-[var(--muted)]"
                    >
                        Read
                    </p>
                    {#if data.user}<a
                            href="/dashboard"
                            class="block font-bold uppercase hover:text-[var(--accent)]"
                            >Dashboard</a
                        >{/if}
                    <a
                        href="/trending"
                        class="block font-bold uppercase hover:text-[var(--accent)]"
                        >Trending</a
                    >
                    <a
                        href="/articles"
                        class="block font-bold uppercase hover:text-[var(--accent)]"
                        >Articles</a
                    >
                </div>
                <div class="space-y-1.5">
                    <p
                        class="text-[0.65rem] font-extrabold uppercase tracking-widest text-[var(--muted)]"
                    >
                        Library
                    </p>
                    {#if data.user}
                        <a
                            href="/feeds"
                            class="block font-bold uppercase hover:text-[var(--accent)]"
                            >Feeds</a
                        >
                        <a
                            href="/library"
                            class="block font-bold uppercase hover:text-[var(--accent)]"
                            >Annotations</a
                        >
                    {/if}
                </div>
                <div class="space-y-1.5">
                    <p
                        class="text-[0.65rem] font-extrabold uppercase tracking-widest text-[var(--muted)]"
                    >
                        Settings
                    </p>
                    <ThemeToggle block={true} />
                    <button
                        class="block font-bold uppercase hover:text-[var(--accent)]"
                        onclick={() => (showShortcuts = true)}>Shortcuts</button
                    >
                    <button
                        class="block font-bold uppercase hover:text-[var(--accent)]"
                        onclick={() => (showInstall = true)}>Install App</button
                    >
                    <a
                        href="/terms"
                        class="block font-bold uppercase hover:text-[var(--accent)]"
                        >Terms</a
                    >
                </div>
            </div>
        </div>
        <div class="border-t-2 border-[var(--border)]">
            <div
                class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-2 px-4 py-3 text-[0.65rem] uppercase tracking-wide text-[var(--muted)]"
            >
                <span>&copy; {new Date().getFullYear()} Glean.at</span>
                <span
                    >Made in Europe · <a
                        href="https://bsky.app/profile/julien.rbrt.fr"
                        class="hover:text-[var(--fg)]">julien.rbrt.fr</a
                    ></span
                >
            </div>
        </div>
    </footer>
</div>

<ShortcutsDialog open={showShortcuts} onClose={() => (showShortcuts = false)} />
<InstallDialog open={showInstall} onClose={() => (showInstall = false)} />
