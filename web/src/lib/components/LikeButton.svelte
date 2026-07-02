<script lang="ts">
    import Icon from "./Icon.svelte";
    import { endpoints } from "$lib/api";

    interface Props {
        articleId: number;
        liked: boolean;
        count: number;
    }
    let {
        articleId,
        liked = $bindable(false),
        count = $bindable(0),
    }: Props = $props();

    let loading = $state(false);

    async function toggle() {
        if (loading) return;
        loading = true;
        const prevLiked = liked;
        const prevCount = count;
        liked = !liked;
        count += liked ? 1 : -1;
        try {
            const res = await endpoints.toggleLike(articleId);
            liked = res.liked;
            count = res.like_count;
        } catch {
            liked = prevLiked;
            count = prevCount;
        } finally {
            loading = false;
        }
    }
</script>

<button
    onclick={toggle}
    disabled={loading}
    title={liked ? "Unlike" : "Like"}
    class="chip"
>
    <Icon name="heart" class="h-3 w-3" fill={liked ? "currentColor" : "none"} />
    <span>{count}</span>
</button>
