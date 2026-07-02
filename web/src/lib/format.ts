export function formatDate(t: string | null | undefined): string {
	if (!t) return '';
	const d = new Date(t);
	if (isNaN(d.getTime())) return '';
	return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit', year: 'numeric' });
}

export function formatDateTime(t: string | number | null | undefined): string {
	if (t == null || t === '') return '';
	const d = typeof t === 'number' ? new Date(t * 1000) : new Date(t);
	if (isNaN(d.getTime())) return '';
	return d.toLocaleString('en-US', {
		month: 'short',
		day: '2-digit',
		year: 'numeric',
		hour: '2-digit',
		minute: '2-digit'
	});
}

export function plainText(html: string): string {
	if (!html) return '';
	const s = html
		.replace(/<script[^>]*>[\s\S]*?<\/script>/gi, ' ')
		.replace(/<[^>]+>/g, ' ')
		.replace(/&amp;/g, '&')
		.replace(/&lt;/g, '<')
		.replace(/&gt;/g, '>')
		.replace(/&quot;/g, '"')
		.replace(/&#39;/g, "'")
		.replace(/&apos;/g, "'")
		.replace(/&nbsp;/g, ' ')
		.replace(/&hellip;/g, '...')
		.replace(/&mdash;/g, '—')
		.replace(/&ndash;/g, '–')
		.replace(/&rsquo;/g, '’')
		.replace(/&lsquo;/g, '‘')
		.replace(/&rdquo;/g, '"')
		.replace(/&ldquo;/g, '"')
		.replace(/&#\d+;/g, '')
		.replace(/\s+/g, ' ')
		.trim();
	return s;
}

const YOUTUBE_HOSTS = ['www.youtube.com', 'youtube.com', 'm.youtube.com', 'youtu.be'];
const EMBED_HOSTS = [
	'www.youtube.com',
	'youtube.com',
	'm.youtube.com',
	'youtu.be',
	'vimeo.com',
	'player.vimeo.com',
	'open.spotify.com',
	'embed.spotify.com',
	'w.soundcloud.com',
	'bandcamp.com'
];

export function youtubeID(rawURL: string): string {
	if (!rawURL) return '';
	let u: URL;
	try {
		u = new URL(rawURL);
	} catch {
		return '';
	}
	const host = u.hostname.toLowerCase();
	if (host === 'youtu.be') {
		return u.pathname.slice(1);
	}
	if (host === 'www.youtube.com' || host === 'youtube.com' || host === 'm.youtube.com') {
		if (u.pathname === '/watch' || u.pathname === '/watch/') {
			return u.searchParams.get('v') ?? '';
		}
		if (u.pathname.startsWith('/embed/')) return u.pathname.slice('/embed/'.length);
		if (u.pathname.startsWith('/shorts/')) return u.pathname.slice('/shorts/'.length);
	}
	return '';
}

export function isEmbedURL(rawURL: string): boolean {
	if (!rawURL) return false;
	try {
		const u = new URL(rawURL);
		return EMBED_HOSTS.includes(u.hostname.toLowerCase());
	} catch {
		return false;
	}
}

export { YOUTUBE_HOSTS };
