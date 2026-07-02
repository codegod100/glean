import adapter from "@sveltejs/adapter-node";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter(),
    // CSRF is enforced by the Go API (double-submit glean_csrf cookie +
    // X-CSRF-Token header). This app has no SvelteKit form actions; the
    // /api/* POSTs proxied here are form-encoded, which SvelteKit's own
    // origin check rejects whenever its computed url.origin differs from the
    // browser's (e.g. behind a TLS-terminating proxy). Trust all origins so
    // those requests reach the API, where Go's CSRF check applies.
    csrf: {
      trustedOrigins: ["*"],
    },
  },
};

export default config;
