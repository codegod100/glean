<script lang="ts">
    import { navigating } from "$app/state";

    // Avoid flashing on instant navigations: only show the bar after a short
    // delay if the navigation is still in progress.
    let visible = $state(false);
    let timer: ReturnType<typeof setTimeout> | undefined;

    $effect(() => {
        const active = navigating.to !== null;
        if (active) {
            timer = setTimeout(() => (visible = true), 100);
        } else {
            if (timer) clearTimeout(timer);
            visible = false;
        }
    });
</script>

{#if visible}
    <div
        class="nav-progress"
        role="progressbar"
        aria-label="Loading"
        aria-busy="true"
    ></div>
{/if}

<style>
    .nav-progress {
        position: fixed;
        top: 0;
        left: 0;
        height: 3px;
        width: 100%;
        background: var(--accent);
        z-index: 60;
        /* Indeterminate sweep: shrink-grow across the viewport. */
        transform-origin: left center;
        animation: nav-progress-indeterminate 1s ease-in-out infinite;
    }

    @keyframes nav-progress-indeterminate {
        0% {
            transform: scaleX(0);
            opacity: 0.85;
        }
        50% {
            transform: scaleX(0.7);
            opacity: 1;
        }
        100% {
            transform: scaleX(0);
            opacity: 0.85;
        }
    }

    @media (prefers-reduced-motion: reduce) {
        .nav-progress {
            animation: none;
            transform: scaleX(1);
            opacity: 1;
        }
    }
</style>
