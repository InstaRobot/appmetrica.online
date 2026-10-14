import { randomBytes } from "node:crypto";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { gzipSync } from "node:zlib";
import { afterEach, describe, expect, it } from "vitest";
import { BUDGET_BYTES, checkBundleSize } from "./check-bundle-size";

// Синтетический вывод `next build` (Turbopack): .next/app-path-routes-manifest.json и
// .next/diagnostics/route-bundle-stats.json плюс чанки в .next/static/chunks.
type Fixture = {
  appPaths: Record<string, string>;
  stats: { route: string; firstLoadChunkPaths: string[] }[];
  chunks: Record<string, Buffer>;
};

const dirs: string[] = [];

function writeBuild({ appPaths, stats, chunks }: Partial<Fixture>): string {
  const dir = mkdtempSync(path.join(tmpdir(), "bundle-size-"));
  dirs.push(dir);
  mkdirSync(path.join(dir, ".next/diagnostics"), { recursive: true });
  mkdirSync(path.join(dir, ".next/static/chunks"), { recursive: true });
  if (appPaths) {
    writeFileSync(path.join(dir, ".next/app-path-routes-manifest.json"), JSON.stringify(appPaths));
  }
  if (stats) {
    writeFileSync(
      path.join(dir, ".next/diagnostics/route-bundle-stats.json"),
      JSON.stringify(stats.map((s) => ({ ...s, firstLoadUncompressedJsBytes: 0 }))),
    );
  }
  for (const [name, content] of Object.entries(chunks ?? {})) {
    writeFileSync(path.join(dir, ".next/static/chunks", name), content);
  }
  return dir;
}

const chunk = (name: string) => `.next/static/chunks/${name}`;
// Случайные байты почти не сжимаются: размер gzip ≈ размеру чанка.
const incompressible = (bytes: number) => randomBytes(bytes);

const APP_PATHS = {
  "/(auth)/login/page": "/login",
  "/(dashboard)/page": "/",
  "/(dashboard)/apps/[appId]/page": "/apps/[appId]",
  "/(dashboard)/settings/page": "/settings",
  "/_not-found/page": "/_not-found",
  "/api/health/route": "/api/health",
};

function run(dir: string) {
  const lines: string[] = [];
  const code = checkBundleSize(dir, (line) => lines.push(line));
  return { code, output: lines.join("\n") };
}

afterEach(() => {
  for (const dir of dirs.splice(0)) rmSync(dir, { recursive: true, force: true });
});

describe("checkBundleSize", () => {
  it("passes when every dashboard page fits the budget", () => {
    const framework = incompressible(120_000);
    const charts = incompressible(100_000);
    const dir = writeBuild({
      appPaths: APP_PATHS,
      stats: [
        { route: "/", firstLoadChunkPaths: [chunk("framework.js")] },
        {
          route: "/apps/[appId]",
          firstLoadChunkPaths: [chunk("framework.js"), chunk("charts.js")],
        },
        { route: "/settings", firstLoadChunkPaths: [chunk("framework.js")] },
        { route: "/login", firstLoadChunkPaths: [chunk("framework.js")] },
      ],
      chunks: { "framework.js": framework, "charts.js": charts },
    });

    const { code, output } = run(dir);

    expect(code).toBe(0);
    expect(output).toMatch(/\/apps\/\[appId\]\s+\d+(\.\d)? KB/);
    expect(output).toContain("/settings");
    expect(output).toContain(`бюджет ${BUDGET_BYTES / 1000} KB gz`);
  });

  it("counts the gzip size of every first-load chunk including shared ones", () => {
    const shared = Buffer.from("shared();".repeat(5_000));
    const own = Buffer.from("own();".repeat(3_000));
    const dir = writeBuild({
      appPaths: { "/(dashboard)/page": "/" },
      stats: [{ route: "/", firstLoadChunkPaths: [chunk("shared.js"), chunk("own.js")] }],
      chunks: { "shared.js": shared, "own.js": own },
    });
    const expected = gzipSync(shared, { level: 9 }).length + gzipSync(own, { level: 9 }).length;

    const { code, output } = run(dir);

    expect(code).toBe(0);
    expect(output).toContain(`${(expected / 1000).toFixed(1)} KB`);
  });

  it("fails with the list of dashboard pages over the budget", () => {
    const dir = writeBuild({
      appPaths: APP_PATHS,
      stats: [
        { route: "/", firstLoadChunkPaths: [chunk("framework.js"), chunk("charts.js")] },
        {
          route: "/apps/[appId]",
          firstLoadChunkPaths: [chunk("framework.js"), chunk("charts.js")],
        },
        { route: "/settings", firstLoadChunkPaths: [chunk("framework.js")] },
      ],
      chunks: {
        "framework.js": incompressible(150_000),
        "charts.js": incompressible(120_000),
      },
    });

    const { code, output } = run(dir);

    expect(code).toBe(1);
    const failed = output.slice(output.indexOf("Превышен бюджет"));
    expect(failed).toContain("/apps/[appId]");
    expect(failed).toMatch(/^\s+\/\s/m);
    expect(failed).not.toContain("/settings");
  });

  it("ignores routes outside the dashboard", () => {
    const dir = writeBuild({
      appPaths: APP_PATHS,
      stats: [
        { route: "/", firstLoadChunkPaths: [chunk("small.js")] },
        { route: "/apps/[appId]", firstLoadChunkPaths: [chunk("small.js")] },
        { route: "/settings", firstLoadChunkPaths: [chunk("small.js")] },
        { route: "/login", firstLoadChunkPaths: [chunk("huge.js")] },
        { route: "/_not-found", firstLoadChunkPaths: [chunk("huge.js")] },
      ],
      chunks: { "small.js": incompressible(1_000), "huge.js": incompressible(300_000) },
    });

    const { code, output } = run(dir);

    expect(code).toBe(0);
    expect(output).not.toContain("/login");
    expect(output).not.toContain("/_not-found");
  });

  it("fails with a hint when the build output is missing", () => {
    const dir = writeBuild({ appPaths: APP_PATHS });

    const { code, output } = run(dir);

    expect(code).toBe(1);
    expect(output).toContain("route-bundle-stats.json");
    expect(output).toContain("pnpm build");
  });

  it("fails when a dashboard page is missing from the build stats", () => {
    const dir = writeBuild({
      appPaths: APP_PATHS,
      stats: [{ route: "/", firstLoadChunkPaths: [chunk("small.js")] }],
      chunks: { "small.js": incompressible(1_000) },
    });

    const { code, output } = run(dir);

    expect(code).toBe(1);
    expect(output).toContain("/apps/[appId]");
    expect(output).toContain("/settings");
  });

  it("fails when the build has no dashboard pages", () => {
    const dir = writeBuild({
      appPaths: { "/(auth)/login/page": "/login" },
      stats: [{ route: "/login", firstLoadChunkPaths: [chunk("small.js")] }],
      chunks: { "small.js": incompressible(1_000) },
    });

    const { code, output } = run(dir);

    expect(code).toBe(1);
    expect(output).toContain("(dashboard)");
  });

  it("fails when a first-load chunk file is missing", () => {
    const dir = writeBuild({
      appPaths: { "/(dashboard)/page": "/" },
      stats: [{ route: "/", firstLoadChunkPaths: [chunk("gone.js")] }],
    });

    const { code, output } = run(dir);

    expect(code).toBe(1);
    expect(output).toContain("gone.js");
  });
});
