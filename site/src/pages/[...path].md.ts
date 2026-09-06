// Every Markdown-backed page has a twin at the same address plus ".md":
// /docs/webhooks.md is the source of /docs/webhooks, for agents and for people who prefer plain text.
import type { APIRoute } from "astro";
import { docs, legal, markdownOf, pathOf, type DocModule } from "../lib/docs";

export function getStaticPaths() {
  return [...docs, ...legal].map((m) => ({ params: { path: pathOf(m).slice(1) }, props: { m } }));
}

export const GET: APIRoute<{ m: DocModule }> = ({ props }) =>
  new Response(markdownOf(props.m), { headers: { "Content-Type": "text/markdown; charset=utf-8" } });
