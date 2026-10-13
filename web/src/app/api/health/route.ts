// Healthcheck контейнера web (Dockerfile HEALTHCHECK, Playwright webServer). Route handler имеет приоритет
// над rewrite `/api/:path*` → Go API, потому что rewrite объявлен в afterFiles (next.config.ts).
export function GET(): Response {
  return Response.json({ status: "ok" });
}
