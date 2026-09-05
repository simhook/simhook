import path from "node:path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Self-contained server for the container image. The tracing root is the
  // monorepo root so workspace packages are included.
  output: "standalone",
  outputFileTracingRoot: path.resolve(process.cwd(), ".."),
};

export default nextConfig;
