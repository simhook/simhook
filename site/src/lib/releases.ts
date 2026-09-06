/**
 * Every release of the repository, read at build time for the changelog.
 * The tag says which component a release belongs to: android-v* is the
 * phone app, sdk-v* and mcp-v* are the npm packages, anything else is the
 * server. A build with no network gets null and links to the releases page.
 */
const RELEASES = "https://api.github.com/repos/simhook/simhook/releases?per_page=100";

export type Release = { tag: string; name: string; published_at: string; html_url: string; body: string; prerelease: boolean };

/** Tag prefixes and the component each one names, in the order the changelog shows them. */
export const COMPONENTS: [prefix: string, label: string][] = [
  ["android-v", "Android app"],
  ["sdk-v", "@simhook/sdk"],
  ["mcp-v", "@simhook/mcp"],
];
export const SERVER = "Server";

export const componentOf = (tag: string): string => COMPONENTS.find(([prefix]) => tag.startsWith(prefix))?.[1] ?? SERVER;

/** The version a tag names: "android-v0.1.2" -> "0.1.2", "v1.4.0" -> "1.4.0". */
export const versionOf = (tag: string): string => tag.replace(/^[a-z]+-v|^v/, "");

export async function listReleases(): Promise<Release[] | null> {
  try {
    const r = await fetch(RELEASES, { headers: { Accept: "application/vnd.github+json", "User-Agent": "simhook.dev-build" } });
    if (!r.ok) return null;
    const rows = (await r.json()) as Record<string, unknown>[];
    if (!Array.isArray(rows)) return null;
    return rows
      .filter((x) => !x.draft)
      .map((x) => ({
        tag: String(x.tag_name ?? ""),
        name: String(x.name ?? x.tag_name ?? ""),
        published_at: String(x.published_at ?? ""),
        html_url: String(x.html_url ?? ""),
        body: String(x.body ?? ""),
        prerelease: Boolean(x.prerelease),
      }));
  } catch {
    return null;
  }
}
