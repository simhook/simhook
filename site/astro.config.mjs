import { defineConfig } from "astro/config";

// Static output: the whole site is files served by Caddy. No runtime.
export default defineConfig({
  site: "https://simhook.dev",
  output: "static",
  trailingSlash: "never",
  build: { format: "file" },
  markdown: { syntaxHighlight: false },
});
