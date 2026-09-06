import { defineConfig, fontProviders } from "astro/config";
import sitemap from "@astrojs/sitemap";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

// Sitemap lastmod comes only from a Markdown page's `updated` frontmatter.
// A wrong date is worse than none, so pages without one get no lastmod.
function updatedDates(dir = "src/pages", out = {}) {
  for (const name of readdirSync(dir)) {
    const file = join(dir, name);
    if (statSync(file).isDirectory()) {
      updatedDates(file, out);
      continue;
    }
    if (!name.endsWith(".md")) continue;
    const m = readFileSync(file, "utf8").match(/^---[\s\S]*?\nupdated:\s*"?(\d{4}-\d{2}-\d{2})"?[\s\S]*?\n---/);
    if (!m) continue;
    const path = "/" + relative("src/pages", file).replace(/\\/g, "/").replace(/\.md$/, "").replace(/(^|\/)index$/, "");
    out[path || "/"] = m[1];
  }
  return out;
}
const dates = updatedDates();

// Static output: the whole site is files served by Caddy. No runtime.
export default defineConfig({
  site: "https://simhook.dev",
  output: "static",
  trailingSlash: "never",
  build: { format: "file" },
  markdown: { syntaxHighlight: false },
  integrations: [
    sitemap({
      filter: (page) => !/\/404$/.test(page),
      serialize(item) {
        const path = new URL(item.url).pathname.replace(/\/$/, "") || "/";
        return dates[path] ? { ...item, lastmod: dates[path] } : item;
      },
    }),
  ],
  // Self-hosted, hashed, preloaded, with metric-matched fallbacks so the swap
  // from the fallback face causes no layout shift. The files are fetched from
  // fontsource once at build and cached in node_modules/.astro/fonts.
  fonts: [
    {
      provider: fontProviders.fontsource(),
      name: "Instrument Sans",
      cssVariable: "--font-sans",
      weights: ["400 700"],
      styles: ["normal"],
      subsets: ["latin", "latin-ext"],
      fallbacks: ["system-ui", "sans-serif"],
    },
    {
      provider: fontProviders.fontsource(),
      name: "Geist Mono",
      cssVariable: "--font-mono",
      weights: ["400 700"],
      styles: ["normal"],
      subsets: ["latin", "latin-ext"],
      fallbacks: ["ui-monospace", "monospace"],
    },
  ],
});
