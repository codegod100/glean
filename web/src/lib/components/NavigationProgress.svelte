<script lang="ts">
    import { navigating } from "$app/state";

    // Monotonic progress bar: the fill only ever moves forward while a
    // navigation is active, then completes to 100% and fades out. This avoids
    // the back-and-forth jitter caused by restarting a looping keyframe
    // animation on every navigation.
    let progress = $state(0);
    let visible = $state(false);
    let fading = $state(false);

    let showTimer: ReturnType<typeof setTimeout> | undefined;
    let trickleTimer: ReturnType<typeof setInterval> | undefined;
    let completeTimer: ReturnType<typeof setTimeout> | undefined;

    function clearTimers() {
        if (showTimer) clearTimeout(showTimer);
        if (trickleTimer) clearInterval(trickleTimer);
        if (completeTimer) clearTimeout(completeTimer);
        showTimer = trickleTimer = completeTimer = undefined;
    }

    function start() {
        clearTimers();
        progress = 0;
        fading = false;
        // Avoid flashing on instant navigations: only show after a delay.
        showTimer = setTimeout(() => {
            visible = true;
            progress = 0.2;
            // Asymptotic trickle toward 0.9 so the bar never stalls at 100%
            // before the navigation actually completes.
            trickleTimer = setInterval(() => {
                progress = Math.min(0.9, progress + (0.9 - progress) * 0.1);
            }, 200);
        }, 100);
    }

    function done() {
        clearTimers();
        if (!visible) return;
        progress = 1;
        fading = true;
        completeTimer = setTimeout(() => {
            visible = false;
            fading = false;
        }, 300);
    }

    $effect(() => {
        if (navigating.to !== null) start();
        else done();
    });

    $effect(() => () => clearTimers());
</script>

{#if visible}
    <div
        class="nav-progress"
        class:fading
        role="progressbar"
        aria-label="Loading"
        aria-busy="true"
        style="transform: scaleX({progress})"
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
        transform-origin: left center;
        opacity: 1;
        z-index: 60;
        transition:
            transform 0.2s ease,
            opacity 0.3s ease;
    }

    .nav-progress.fading {
        opacity: 0;
    }

    @media (prefers-reduced-motion: reduce) {
        .nav-progress {
            transition: none;
        }
    }
</style>
