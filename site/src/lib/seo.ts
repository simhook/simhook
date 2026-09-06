/**
 * What every page tells machines: the one place the brand suffix is added,
 * the share-image rule, and the JSON-LD nodes. Nothing here invents a fact:
 * dates come from frontmatter, versions from the release manifest, and there
 * are no ratings or offers that cannot be bought.
 */
import { readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { publicDir } from "astro:config/server";
import type { Manifest } from "./release";

export const SITE = "https://simhook.dev";
export const BRAND = "simhook";
export const GITHUB = "https://github.com/simhook/simhook";
export const CONTACT_EMAIL = "hello@simhook.dev";
export const ORG_ID = `${SITE}/#org`;
export const SITE_ID = `${SITE}/#website`;
export const DEFAULT_DESCRIPTION =
  "Turn an Android phone into an SMS API. Send and receive texts from your code through your own SIM, with webhooks, an SDK, and an MCP server.";

/** "/docs/sending.html", "/docs/index", "/docs/" and "/docs" are one page. */
export const normalizePath = (p: string) => p.replace(/\.html$/, "").replace(/\/index$/, "").replace(/\/$/, "") || "/";

/** The brand suffix is added here and nowhere else. */
export const fullTitle = (title: string) => `${title} · ${BRAND}`;

export const appUrl = () => (import.meta.env.PUBLIC_APP_URL as string | undefined) || "https://app.simhook.dev";
export const apiUrl = () => (import.meta.env.PUBLIC_API_URL as string | undefined) || "https://api.simhook.dev";

/** "/" -> "home", "/docs/guides/two-way-sms" -> "docs-guides-two-way-sms". Shared with scripts/brand.mjs. */
export const ogSlug = (path: string) => (path === "/" ? "home" : path.slice(1).replace(/\//g, "-"));

const ogAvailable = (() => {
  try {
    return new Set(readdirSync(fileURLToPath(new URL("og/", publicDir))).map((f) => f.replace(/\.png$/, "")));
  } catch {
    return new Set<string>();
  }
})();

/** The page's share image if the brand script produced one, otherwise the default. */
export function ogImageFor(path: string): string {
  const slug = ogSlug(path);
  return `/og/${ogAvailable.has(slug) ? slug : "default"}.png`;
}

/** Frontmatter dates arrive as strings when quoted and as Date objects when not; pages want YYYY-MM-DD. */
export const isoDate = (v: unknown): string | undefined =>
  v instanceof Date ? v.toISOString().slice(0, 10) : typeof v === "string" && v ? v.slice(0, 10) : undefined;

/** Google wants dates in structured data as full timestamps with a zone; a page's `updated` day becomes midnight UTC. */
export const isoDateTime = (day: string) => `${day}T00:00:00+00:00`;

export const formatUpdated = (iso: string) =>
  new Date(`${iso}T00:00:00Z`).toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });

type Node = Record<string, unknown>;

/** Present on every page so `@id` references resolve wherever a crawler lands. */
export const orgNode = (): Node => ({
  "@type": "Organization",
  "@id": ORG_ID,
  name: BRAND,
  url: `${SITE}/`,
  logo: { "@type": "ImageObject", url: `${SITE}/brand/logo-512.png`, width: 512, height: 512 },
  description: "simhook turns an Android phone into an SMS API: send and receive texts from code through your own SIM, with webhooks, a TypeScript SDK, and an MCP server.",
  email: CONTACT_EMAIL,
  founder: { "@type": "Person", name: "Enes" },
  sameAs: ["https://github.com/simhook", "https://www.npmjs.com/~simhook"],
});

/** Home page only: tells search engines the site's name. */
export const websiteNode = (description: string): Node => ({
  "@type": "WebSite",
  "@id": SITE_ID,
  url: `${SITE}/`,
  name: BRAND,
  description,
  inLanguage: "en",
  publisher: { "@id": ORG_ID },
});

export const breadcrumbs = (items: [string, string][]): Node => ({
  "@type": "BreadcrumbList",
  itemListElement: items.map(([name, url], i) => ({ "@type": "ListItem", position: i + 1, name, item: `${SITE}${url}` })),
});

export const techArticle = (o: { url: string; title: string; description?: string; updated?: string; image?: string }): Node => ({
  "@type": "TechArticle",
  "@id": `${SITE}${o.url}#article`,
  headline: o.title,
  ...(o.description ? { description: o.description } : {}),
  url: `${SITE}${o.url}`,
  mainEntityOfPage: `${SITE}${o.url}`,
  ...(o.image ? { image: [`${SITE}${o.image}`] } : {}),
  ...(o.updated ? { dateModified: isoDateTime(o.updated) } : {}),
  inLanguage: "en",
  author: { "@id": ORG_ID },
  publisher: { "@id": ORG_ID },
  isPartOf: { "@id": SITE_ID },
});

export const faqPage = (qa: { q: string; a: string }[]): Node => ({
  "@type": "FAQPage",
  mainEntity: qa.map(({ q, a }) => ({ "@type": "Question", name: q, acceptedAnswer: { "@type": "Answer", text: a } })),
});

/** The Android app. No rating or review exists, so this describes the app rather than asking for a rich result. */
export const softwareApp = (m: Manifest | null): Node => ({
  "@type": "SoftwareApplication",
  name: "simhook for Android",
  url: `${SITE}/download`,
  operatingSystem: "Android 8.0 or newer",
  applicationCategory: "DeveloperApplication",
  isAccessibleForFree: true,
  offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
  downloadUrl: `${SITE}/download/simhook.apk`,
  installUrl: `${SITE}/download/simhook.apk`,
  license: "https://www.gnu.org/licenses/agpl-3.0.html",
  screenshot: `${SITE}/img/app-home.webp`,
  author: { "@id": ORG_ID },
  ...(m
    ? {
        softwareVersion: m.version_name,
        fileSize: `${(m.size_bytes / 1048576).toFixed(1)}MB`,
        ...(m.released_at ? { datePublished: m.released_at } : {}),
        ...(m.notes ? { releaseNotes: m.notes } : {}),
      }
    : {}),
});

export const webPage = (o: { url: string; title: string; description?: string; updated?: string; type?: string }): Node => ({
  "@type": o.type ?? "WebPage",
  "@id": `${SITE}${o.url}#page`,
  url: `${SITE}${o.url}`,
  name: o.title,
  ...(o.description ? { description: o.description } : {}),
  ...(o.updated ? { dateModified: isoDateTime(o.updated) } : {}),
  isPartOf: { "@id": SITE_ID },
  about: { "@id": ORG_ID },
});
