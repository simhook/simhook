// /llms.txt: an index of the site for AI agents, in the llmstxt.org shape.
// Search engines are told to ignore it (X-Robots-Tag in the Caddyfile); the HTML pages are what they index.
import type { APIRoute } from "astro";
import { docs, GUIDES, NAV, pathOf } from "../lib/docs";
import { GITHUB, SITE } from "../lib/seo";

const line = (path: string, label: string, description?: string) =>
  `- [${label}](${SITE}${path === "/docs" ? "/docs" : path}.md)${description ? `: ${description}` : ""}`;

const byPath = new Map(docs.map((m) => [pathOf(m), m]));
const section = (title: string, entries: [string, string][]) => {
  const present = entries.filter(([p]) => byPath.has(p));
  if (present.length === 0) return "";
  return `## ${title}\n\n${present.map(([p, label]) => line(p, label, byPath.get(p)?.frontmatter.description)).join("\n")}\n\n`;
};

const text = `# simhook

> simhook turns an Android phone into an SMS API. Send and receive texts from code through the phone's own SIM: a REST API at https://api.simhook.dev (API key in the X-Api-Key header), signed webhooks, a TypeScript SDK, and an MCP server. Open source under AGPL-3.0 and self-hostable.

Every documentation page below is available as Markdown at the same address with .md appended. The whole documentation set is one file at ${SITE}/llms-full.txt.

${section("Docs", NAV)}${section("Guides", GUIDES)}## Reference

- [API reference](${SITE}/docs/api): every endpoint, generated from the OpenAPI document
- [OpenAPI document](https://api.simhook.dev/openapi.json): the machine-readable contract the API serves
- [OpenAPI document in the repository](${GITHUB}/blob/main/packages/contracts/openapi.json)
- [API catalog](${SITE}/.well-known/api-catalog): an RFC 9727 linkset naming the API, its OpenAPI document, its docs, and its health endpoint
- [Agent skill](${SITE}/.well-known/agent-skills/simhook/SKILL.md): instructions an agent can install to send and receive SMS with simhook; the index with its digest is at ${SITE}/.well-known/agent-skills/index.json

## Packages

- [@simhook/sdk on npm](https://www.npmjs.com/package/@simhook/sdk): the TypeScript client
- [@simhook/mcp on npm](https://www.npmjs.com/package/@simhook/mcp): the MCP server
- [Source on GitHub](${GITHUB}): API, dashboard, Android app, deployment files

## Optional

- [Pricing](${SITE}/pricing)
- [Download the Android app](${SITE}/download)
- [Compare with hosted SMS APIs](${SITE}/compare)
- [About](${SITE}/about)
- [Changelog](${SITE}/changelog)
- [Privacy](${SITE}/privacy.md)
- [Terms](${SITE}/terms.md)
`;

export const GET: APIRoute = () => new Response(text, { headers: { "Content-Type": "text/plain; charset=utf-8" } });
