import type { RequestHandler } from "./$types";

export const GET: RequestHandler = ({ url }) => {
	const body = [
		"User-agent: *",
		"Disallow: /dashboard",
		"Disallow: /articles",
		"Disallow: /feeds",
		"Disallow: /library",
		"Disallow: /profile",
		"Disallow: /stats",
		"",
		`Sitemap: ${url.origin}/sitemap.xml`,
		"",
	].join("\n");
	return new Response(body, {
		headers: { "content-type": "text/plain; charset=utf-8" },
	});
};
