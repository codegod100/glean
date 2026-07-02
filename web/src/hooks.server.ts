import type { Handle } from "@sveltejs/kit";

const API_URL = process.env.GLEAN_API_URL ?? "http://localhost:8080";

async function proxyApi({
  request,
  url,
}: {
  request: Request;
  url: URL;
}): Promise<Response> {
  const target = API_URL.replace(/\/$/, "") + url.pathname + url.search;

  const headers = new Headers(request.headers);
  // Forward the browser's original Host so the Go CSRF same-origin check
  // (Origin vs Host) passes. Go routes by path, not Host.
  const originHost = request.headers.get("host");
  if (originHost) {
    headers.set("host", originHost);
    headers.set("x-forwarded-host", originHost);
  }
  const proto = url.protocol.replace(":", "");
  headers.set("x-forwarded-proto", proto);

  const init: RequestInit = {
    method: request.method,
    headers,
    body:
      request.method !== "GET" && request.method !== "HEAD"
        ? await request.arrayBuffer()
        : undefined,
    // @ts-expect-error Node fetch supports duplex for streaming request bodies.
    duplex: "half",
  };

  const upstream = await fetch(target, init);

  const respHeaders = new Headers();
  for (const [key, value] of upstream.headers.entries()) {
    const lower = key.toLowerCase();
    if (
      lower === "transfer-encoding" ||
      lower === "content-encoding" ||
      lower === "content-length"
    )
      continue;
    respHeaders.set(key, value);
  }

  return new Response(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: respHeaders,
  });
}

export const handle: Handle = async ({ event, resolve }) => {
  if (event.url.pathname.startsWith("/api/") || event.url.pathname === "/api") {
    return proxyApi(event);
  }

  // Populate locals for the layout using the per-request fetch.
  try {
    const res = await event.fetch("/api/me");
    if (res.ok) {
      const data = await res.json();
      event.locals.user = data.user ?? null;
      event.locals.csrfToken = data.csrf_token ?? "";
    }
  } catch {
    // API unavailable; continue as anonymous.
  }

  return resolve(event);
};
