import { createMDX } from "fumadocs-mdx/next";
import type { NextConfig } from "next";

// rewrites фиксируются на этапе `next build`: API_INTERNAL_URL передаётся как build-arg образа.
const apiInternalUrl = process.env.API_INTERNAL_URL ?? "http://localhost:8080";

const securityHeaders = [
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), payment=()" },
  { key: "Strict-Transport-Security", value: "max-age=63072000; includeSubDomains" },
];

const nextConfig: NextConfig = {
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
  typedRoutes: true,
  cacheComponents: true,
  async rewrites() {
    return {
      beforeFiles: [],
      // afterFiles: собственные route handlers web (например /api/health) имеют приоритет над прокси
      afterFiles: [{ source: "/api/:path*", destination: `${apiInternalUrl}/:path*` }],
      fallback: [],
    };
  },
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
};

const withMDX = createMDX();

export default withMDX(nextConfig);
