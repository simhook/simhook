#!/usr/bin/env node
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { Simhook, SimhookError } from "@simhook/sdk";
import { createServer } from "./server";
import { VERSION } from "./version";

const USAGE = `simhook-mcp ${VERSION}
MCP server that lets AI agents send and read SMS through simhook. Speaks MCP over stdio.

Environment:
  SIMHOOK_API_KEY   API key from the simhook dashboard (required)
  SIMHOOK_BASE_URL  API origin for self-hosted installs (optional)

Options:
  -h, --help      Show this help
  -v, --version   Print the version
`;

const args = process.argv.slice(2);
if (args.includes("-h") || args.includes("--help")) {
  process.stdout.write(USAGE);
  process.exit(0);
}
if (args.includes("-v") || args.includes("--version")) {
  process.stdout.write(`${VERSION}\n`);
  process.exit(0);
}

let client: Simhook;
try {
  client = new Simhook();
} catch (err) {
  process.stderr.write(`simhook-mcp: ${err instanceof SimhookError ? err.message : String(err)}\n`);
  process.exit(1);
}

const server = createServer({ client });
await server.connect(new StdioServerTransport());
// stdout carries the protocol, so diagnostics go to stderr.
process.stderr.write(`simhook-mcp ${VERSION} connected to ${client.baseUrl}\n`);
