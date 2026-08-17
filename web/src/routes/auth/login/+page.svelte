<script lang="ts">
    import { endpoints } from "$lib/api";
    import Logo from "$lib/components/Logo.svelte";
    import Seo from "$lib/components/Seo.svelte";
    import type { Actor } from "$lib/types";

    let handle = $state("");
    let actors = $state<Actor[]>([]);
    let selected = $state(-1);
    let timer: ReturnType<typeof setTimeout>;
    let err = $state("");

    function onInput() {
        clearTimeout(timer);
        const q = handle.trim().replace(/^@/, "");
        if (q.length < 1) {
            actors = [];
            selected = -1;
            return;
        }
        timer = setTimeout(async () => {
            try {
                const r = await endpoints.authActors(q);
                actors = r.actors ?? [];
                selected = -1;
            } catch {
                actors = [];
            }
        }, 250);
    }

    function onKeydown(e: KeyboardEvent) {
        if (!actors.length) return;
        if (e.key === "ArrowDown") {
            e.preventDefault();
            selected = Math.min(selected + 1, actors.length - 1);
        } else if (e.key === "ArrowUp") {
            e.preventDefault();
            selected = Math.max(selected - 1, 0);
        } else if (e.key === "Enter" && selected >= 0) {
            e.preventDefault();
            pick(actors[selected]);
        } else if (e.key === "Escape") {
            actors = [];
        }
    }

    function pick(a: Actor) {
        handle = a.handle;
        actors = [];
    }

    async function submit() {
        const h = handle.trim().replace(/^@/, "");
        if (!h) return;
        try {
            const r = await endpoints.authStart(h);
            window.location.href = r.redirect;
        } catch (e) {
            err = e instanceof Error ? e.message : "Sign in failed";
        }
    }

    async function register() {
        try {
            const r = await endpoints.authRegister();
            window.location.href = r.redirect;
        } catch (e) {
            err = e instanceof Error ? e.message : "Registration failed";
        }
    }
</script>

<Seo
    title="Sign in"
    description="Sign in to Glean with your Bluesky handle or any AT Protocol account."
/>

<div class="flex min-h-screen items-center justify-center px-4 py-12">
    <div class="w-full max-w-sm">
        <div class="mb-8 flex justify-center">
            <Logo size="lg" />
        </div>

        <div class="panel p-6">
            <h1
                class="text-center text-2xl font-extrabold uppercase tracking-tight"
            >
                Welcome
            </h1>
            <p
                class="mt-1 text-center text-xs uppercase tracking-widest text-[var(--muted)]"
            >
                The social RSS reader on AT Protocol.
            </p>

            <form
                onsubmit={(e) => {
                    e.preventDefault();
                    submit();
                }}
                id="login-form"
                class="mt-6 space-y-3"
            >
                <label
                    for="handle-input"
                    class="block text-xs font-extrabold uppercase tracking-widest text-[var(--muted)]"
                >
                    Account handle
                </label>
                <div class="relative">
                    <input
                        bind:value={handle}
                        oninput={onInput}
                        onkeydown={onKeydown}
                        type="text"
                        placeholder="you.bsky.social"
                        id="handle-input"
                        class="input-brutal"
                        autocomplete="off"
                    />
                    <div
                        class="absolute left-0 right-0 top-full z-50 panel divide-y-2 divide-[var(--border)] {actors.length
                            ? ''
                            : 'hidden'}"
                    >
                        {#each actors as a, i}
                            <button
                                type="button"
                                onclick={() => pick(a)}
                                class="flex w-full items-center gap-3 p-2.5 text-left {i ===
                                selected
                                    ? 'bg-[var(--bg)]'
                                    : ''}"
                            >
                                {#if a.avatar}
                                    <img
                                        src={a.avatar}
                                        class="h-8 w-8 shrink-0 border-2 border-[var(--border)] object-cover"
                                        alt=""
                                    />
                                {:else}
                                    <div
                                        class="h-8 w-8 shrink-0 border-2 border-[var(--border)] bg-[var(--surface)]"
                                    ></div>
                                {/if}
                                <div class="min-w-0">
                                    <div class="truncate text-sm font-bold">
                                        @{a.handle}
                                    </div>
                                    {#if a.displayName}
                                        <div
                                            class="truncate text-xs text-[var(--muted)]"
                                        >
                                            {a.displayName}
                                        </div>
                                    {/if}
                                </div>
                            </button>
                        {/each}
                    </div>
                </div>

                <button type="submit" class="btn btn-accent w-full"
                    >Login</button
                >
            </form>

            {#if err}
                <p
                    class="mt-4 text-center text-xs font-bold uppercase tracking-wide text-[var(--danger)]"
                >
                    {err}
                </p>
            {/if}
        </div>

        <div
            class="my-6 flex items-center gap-3 text-xs uppercase tracking-widest text-[var(--muted)]"
        >
            <span class="h-0 flex-1 border-t-2 border-[var(--border)]"></span>
            <span>New here?</span>
            <span class="h-0 flex-1 border-t-2 border-[var(--border)]"></span>
        </div>

        <button type="button" onclick={register} class="btn w-full">
            Register with Eurosky
        </button>

        <p
            class="mt-8 text-center text-[11px] leading-relaxed text-[var(--muted)]"
        >
            By continuing, you agree to the
            <a
                href="/terms"
                class="font-bold text-[var(--accent)] underline underline-offset-2"
                >Terms of Service</a
            >.
        </p>
    </div>
</div>
