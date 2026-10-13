// @vitest-environment node
import { describe, expect, it } from "vitest";
import nextConfig from "../../../../next.config";
import { GET } from "./route";

describe("GET /api/health", () => {
  it("returns ok", async () => {
    const res = GET();

    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toContain("application/json");
    expect(await res.json()).toEqual({ status: "ok" });
  });

  it("is not shadowed by the /api/* rewrite", async () => {
    const rewrites = await nextConfig.rewrites?.();

    expect(Array.isArray(rewrites)).toBe(false);
    if (!rewrites || Array.isArray(rewrites)) throw new Error("rewrites must be an object");
    // Rewrites в beforeFiles перекрыли бы route handlers web; прокси на Go API живёт только в afterFiles.
    expect(rewrites.beforeFiles).toEqual([]);
    expect(rewrites.afterFiles).toContainEqual(expect.objectContaining({ source: "/api/:path*" }));
  });
});
