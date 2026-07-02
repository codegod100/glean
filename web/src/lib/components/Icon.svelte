<script lang="ts">
    // Monochrome line icons sized by `class`. Each path is drawn inside a 24x24 viewBox
    // with stroke=currentColor, fill=none unless `fill` is set.
    interface Props {
        name: string;
        class?: string;
        strokeWidth?: number;
        fill?: string;
    }
    let {
        name,
        class: cls = "h-5 w-5",
        strokeWidth = 1.75,
        fill = "none",
    }: Props = $props();

    const paths: Record<string, string> = {
        grid: "M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z",
        feed: "M4 11a9 9 0 0 1 9 9M4 4a16 16 0 0 1 16 16M6 19a1 1 0 1 0 0-2 1 1 0 0 0 0 2z",
        fire: "M12 2c1 3 4 4 4 8a4 4 0 1 1-8 0c0-2 1-3 2-4-3 0-4 3-4 6a6 6 0 1 0 12 0c0-6-4-8-6-10z",
        note: "M5 3h10l4 4v14H5zM15 3v4h4M9 13h6M9 17h4",
        check: "M4 12l5 5L20 6",
        x: "M6 6l12 12M18 6L6 18",
        chevronDown: "M6 9l6 6 6-6",
        chevronUp: "M6 15l6-6 6 6",
        chevronRight: "M9 6l6 6-6 6",
        arrowLeft: "M19 12H5M12 19l-7-7 7-7",
        arrowRight: "M5 12h14M12 5l7 7-7 7",
        external: "M14 5h5v5M19 5l-9 9M19 14v5H5V5h5",
        search: "M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM21 21l-4.3-4.3",
        heart: "M12 21s-7-4.5-9.5-9C1 9 2.5 5 6 5c2 0 3 1 6 4 3-3 4-4 6-4 3.5 0 5 4 3.5 7-2.5 4.5-9.5 9-9.5 9z",
        heartFill:
            "M12 21s-7-4.5-9.5-9C1 9 2.5 5 6 5c2 0 3 1 6 4 3-3 4-4 6-4 3.5 0 5 4 3.5 7-2.5 4.5-9.5 9-9.5 9z",
        bookmark: "M6 3h12v18l-6-4-6 4z",
        plus: "M12 5v14M5 12h14",
        minus: "M5 12h14",
        logout: "M16 17l5-5-5-5M21 12H9M9 21H4V3h5",
        refresh: "M21 12a9 9 0 1 1-3-6.7M21 4v5h-5",
        edit: "M4 20h4l11-11-4-4L4 16zM14 6l4 4",
        trash: "M4 7h16M9 7V3h6v4M6 7l1 14h10l1-14",
        upload: "M12 16V4M7 9l5-5 5 5M4 20h16",
        download: "M12 4v12M7 11l5 5 5-5M4 20h16",
        keyboard: "M3 6h18v12H3zM7 10h0M11 10h0M15 10h0M7 14h10",
        eye: "M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7zM12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z",
        trending: "M3 17l6-6 4 4 8-8M21 7h-5M21 7v5",
        users: "M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM2 21c0-4 3-7 7-7s7 3 7 7M17 11a3 3 0 1 0 0-6M22 21c0-3-2-5-5-6",
        chart: "M3 21h18M6 21v-6M11 21V8M16 21v-9M21 21V5",
        sun: "M12 4V2M12 22v-2M4 12H2M22 12h-2M6 6L4 4M20 4l-2 2M6 18l-2 2M20 20l-2-2M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z",
        moon: "M21 13a9 9 0 1 1-10-10 7 7 0 0 0 10 10z",
        globe: "M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zM2 12h20M12 2a15 15 0 0 1 0 20M12 2a15 15 0 0 0 0 20",
        sparkles:
            "M12 3l1.8 4.7L18.5 9.5 13.8 11.3 12 16l-1.8-4.7L5.5 9.5l4.7-1.8zM19 14l.9 2.3 2.3.9-2.3.9L19 21l-.9-2.3-2.3-.9 2.3-.9z",
    };
</script>

<svg
    xmlns="http://www.w3.org/2000/svg"
    {fill}
    stroke="currentColor"
    stroke-width={strokeWidth}
    stroke-linecap="square"
    stroke-linejoin="miter"
    viewBox="0 0 24 24"
    class={cls}
    aria-hidden="true"
>
    <path d={paths[name] ?? ""} />
</svg>
