// Бюджет initial JS страниц дашборда (NFR-006, T-044): ≤ 250 KB gzip на страницу.
//
//   pnpm build && pnpm check:bundle      # node scripts/check-bundle-size.ts
//
// Turbopack-сборка Next 16 не печатает «First Load JS», поэтому скрипт читает её диагностику:
// .next/diagnostics/route-bundle-stats.json (чанки первой загрузки каждого маршрута) и
// .next/app-path-routes-manifest.json (какие маршруты лежат в route group (dashboard)).
// Размер страницы — сумма gzip (уровень 9) всех её чанков, включая общие; ленивые чанки
// (next/dynamic, import()) в первую загрузку не входят. Exit 1 — список страниц сверх бюджета.
import { readFileSync } from "node:fs";
import path from "node:path";
import { gzipSync } from "node:zlib";

export const BUDGET_BYTES = 250 * 1000;

const STATS_FILE = ".next/diagnostics/route-bundle-stats.json";
const ROUTES_FILE = ".next/app-path-routes-manifest.json";
const DASHBOARD_PAGE = /^\/\(dashboard\)(\/.*)?\/page$/;

type RouteStats = { route: string; firstLoadChunkPaths: string[] };
type PageSize = { route: string; bytes: number };

function readJson<T>(webDir: string, file: string): T {
  try {
    return JSON.parse(readFileSync(path.join(webDir, file), "utf8")) as T;
  } catch (err) {
    throw new Error(
      `не прочитан ${file} (${(err as Error).message}); сначала выполните pnpm build`,
    );
  }
}

function dashboardRoutes(appPaths: Record<string, string>): string[] {
  const routes = Object.entries(appPaths)
    .filter(([appPath]) => DASHBOARD_PAGE.test(appPath))
    .map(([, route]) => route);
  if (routes.length === 0) throw new Error(`в ${ROUTES_FILE} нет страниц route group (dashboard)`);
  return routes.sort();
}

function measure(webDir: string, routes: string[], stats: RouteStats[]): PageSize[] {
  const byRoute = new Map(stats.map((s) => [s.route, s.firstLoadChunkPaths]));
  const missing = routes.filter((route) => !byRoute.has(route));
  if (missing.length > 0) {
    throw new Error(`в ${STATS_FILE} нет страниц ${missing.join(", ")}: пересоберите pnpm build`);
  }
  const gzipped = new Map<string, number>();
  const gzipSize = (chunk: string): number => {
    let size = gzipped.get(chunk);
    if (size === undefined) {
      let content: Buffer;
      try {
        content = readFileSync(path.join(webDir, chunk));
      } catch {
        throw new Error(`нет чанка ${chunk}: пересоберите pnpm build`);
      }
      size = gzipSync(content, { level: 9 }).length;
      gzipped.set(chunk, size);
    }
    return size;
  };
  return routes.map((route) => ({
    route,
    bytes: (byRoute.get(route) ?? []).reduce((sum, chunk) => sum + gzipSize(chunk), 0),
  }));
}

const kb = (bytes: number) => `${(bytes / 1000).toFixed(1)} KB`;

// checkBundleSize печатает размеры страниц дашборда и возвращает код выхода: 0 — все в бюджете.
export function checkBundleSize(webDir: string, print: (line: string) => void): number {
  let sizes: PageSize[];
  try {
    const routes = dashboardRoutes(readJson<Record<string, string>>(webDir, ROUTES_FILE));
    sizes = measure(webDir, routes, readJson<RouteStats[]>(webDir, STATS_FILE));
  } catch (err) {
    print(`check-bundle-size: ${(err as Error).message}`);
    return 1;
  }

  const width = Math.max(...sizes.map((s) => s.route.length));
  print(`Initial JS страниц дашборда (gzip), бюджет ${BUDGET_BYTES / 1000} KB gz:`);
  for (const { route, bytes } of sizes) {
    print(`  ${route.padEnd(width)}  ${kb(bytes).padStart(9)}${bytes > BUDGET_BYTES ? "  ✗" : ""}`);
  }

  const over = sizes.filter((s) => s.bytes > BUDGET_BYTES);
  if (over.length === 0) return 0;
  print(`Превышен бюджет ${BUDGET_BYTES / 1000} KB gz на ${over.length} стр.:`);
  for (const { route, bytes } of over) {
    print(`  ${route} ${kb(bytes)} (+${kb(bytes - BUDGET_BYTES)})`);
  }
  return 1;
}

if (import.meta.main) {
  process.exitCode = checkBundleSize(process.cwd(), (line) => process.stdout.write(`${line}\n`));
}
