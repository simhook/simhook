// /llms-full.txt: the whole documentation set as one Markdown file, for agents that want everything at once.
import type { APIRoute } from "astro";
import { docs, markdownOf, pathOf } from "../lib/docs";
import { SITE } from "../lib/seo";

const text = [
  `# simhook documentation\n\nEvery documentation page of ${SITE}, in reading order. The API reference is generated from the OpenAPI document at https://api.simhook.dev/openapi.json and is not repeated here.`,
  ...docs.map((m) => `Source: ${SITE}${pathOf(m)}\n\n${markdownOf(m)}`),
].join("\n\n---\n\n");

export const GET: APIRoute = () => new Response(`${text}\n`, { headers: { "Content-Type": "text/plain; charset=utf-8" } });
