/**
 * The one list of documentation pages. The docs nav, the guides row, llms.txt,
 * llms-full.txt, and the Markdown twins all read it, so a page added here shows
 * up everywhere at once.
 */
import { normalizePath, SITE } from "./seo";

export type DocFrontmatter = {
  title: string;
  description?: string;
  /** The <title> and share title, when the H1 alone would not say what the page is about. */
  headTitle?: string;
  /** YYYY-MM-DD of the last real change. Rendered on the page and used as lastmod. A Date when the YAML value is unquoted. */
  updated?: string | Date;
  /** Share-image title override. */
  og?: string;
};

export type DocModule = {
  frontmatter: DocFrontmatter;
  rawContent(): string;
  url: string;
  file: string;
};

const modules = import.meta.glob<DocModule>(["../pages/docs/**/*.md", "../pages/*.md"], { eager: true });

export const NAV: [string, string][] = [
  ["/docs", "Quickstart"],
  ["/docs/sending", "Sending"],
  ["/docs/receiving", "Receiving"],
  ["/docs/webhooks", "Webhooks"],
  ["/docs/sdk", "SDK"],
  ["/docs/mcp", "MCP"],
  ["/docs/self-hosting", "Self-hosting"],
  ["/docs/api", "API reference"],
];

export const GUIDES: [string, string][] = [
  ["/docs/guides/forward-verification-codes", "Verification codes"],
  ["/docs/guides/server-down-sms-alert", "Server alerts"],
  ["/docs/guides/two-way-sms", "Two-way SMS"],
];

const byUrl = new Map(Object.values(modules).map((m) => [normalizePath(m.url), m]));

/** Markdown-backed docs in nav order (the API reference is generated, so it is not here). */
export const docs: DocModule[] = [...NAV, ...GUIDES].map(([p]) => byUrl.get(p)).filter((m): m is DocModule => m !== undefined);
export const legal: DocModule[] = ["/privacy", "/terms"].map((p) => byUrl.get(p)).filter((m): m is DocModule => m !== undefined);

export const pathOf = (m: DocModule) => normalizePath(m.url);
export const hasMarkdownTwin = (path: string) => byUrl.has(path);

/** The page as Markdown: its title, its description as a quote, then the body with links made absolute. */
export function markdownOf(m: DocModule): string {
  const body = m
    .rawContent()
    .replace(/^---[\s\S]*?\n---\s*/, "")
    .replace(/\]\(\//g, `](${SITE}/`)
    .trim();
  const { title, description } = m.frontmatter;
  return `# ${title}\n\n${description ? `> ${description}\n\n` : ""}${body}\n`;
}
