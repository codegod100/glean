import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ fetch }) => {
	const res = await fetch('/api/sitemap');
	const text = await res.text();
	return new Response(text, {
		headers: { 'content-type': 'application/xml' }
	});
};
