import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: {
    // Match the production adapter-node port so dev and prod share an origin.
    port: 3000,
    strictPort: true,
  },
});
