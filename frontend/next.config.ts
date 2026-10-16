import type { NextConfig } from "next";

// Evaluated at build time and baked into the standalone output, so API_URL must
// be present as a build argument in Docker (see docker-compose.yml).
const apiUrl = process.env.API_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  // The browser calls /api/* on this origin and Next forwards it to the Go API,
  // so there is no CORS setup.
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiUrl}/api/:path*` }];
  },
  cacheComponents: true,
  partialPrefetching: true,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
