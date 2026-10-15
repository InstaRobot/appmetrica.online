import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";
import { config, proxy } from "./proxy";

function request(path: string, cookie?: string, acceptLanguage?: string) {
  const headers = new Headers();
  if (cookie) headers.set("cookie", cookie);
  if (acceptLanguage) headers.set("accept-language", acceptLanguage);
  return new NextRequest(new URL(path, "http://localhost:3000"), { headers });
}

const passesThrough = (res: Response) => res.headers.get("x-middleware-next") === "1";
const rewrittenTo = (res: Response) => {
  const target = res.headers.get("x-middleware-rewrite");
  if (!target) return null;
  const url = new URL(target);
  return `${url.pathname}${url.search}`;
};
const savedLocale = (res: Response) =>
  res.headers.get("set-cookie")?.match(/am_lang=(\w+)/)?.[1] ?? null;

describe("proxy", () => {
  it("redirects anonymous users to login with next", () => {
    const res = proxy(request("/apps/x"));

    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toBe("http://localhost:3000/login?next=%2Fapps%2Fx");
  });

  it("keeps the query string in next", () => {
    const res = proxy(request("/apps/x/retention?from=2026-10-01&to=2026-10-12"));

    expect(res.status).toBe(307);
    const location = new URL(res.headers.get("location") ?? "");
    expect(location.pathname).toBe("/login");
    expect(location.searchParams.get("next")).toBe(
      "/apps/x/retention?from=2026-10-01&to=2026-10-12",
    );
  });

  it("redirects the portfolio to login with next", () => {
    const res = proxy(request("/portfolio"));

    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toBe("http://localhost:3000/login?next=%2Fportfolio");
  });

  it("shows the public landing at / without a session", () => {
    expect(rewrittenTo(proxy(request("/")))).toBe("/en");
  });

  it.each(["/login", "/login?next=%2Fapps", "/setup/abc123", "/api/health", "/api/v1/portfolio"])(
    "lets anonymous requests to %s through",
    (path) => {
      expect(passesThrough(proxy(request(path)))).toBe(true);
    },
  );

  it.each(["/demo", "/demo/exit"])("lets anonymous visitors open %s", (path) => {
    expect(passesThrough(proxy(request(path)))).toBe(true);
  });

  it.each(["/portfolio", "/apps/x/retention", "/apps/new", "/settings"])(
    "lets demo visitors open %s without a session",
    (path) => {
      expect(passesThrough(proxy(request(path, "am_demo=1")))).toBe(true);
    },
  );

  it.each(["/docs", "/docs/recipes/ios-swift"])(
    "lets anonymous visitors read the public docs at %s",
    (path) => {
      expect(rewrittenTo(proxy(request(path)))).toBe(`/en${path}`);
    },
  );

  it("passes the docs search through: Fumadocs sends the locale in the query", () => {
    expect(passesThrough(proxy(request("/docs/search?query=events&locale=ru", "am_lang=ru")))).toBe(
      true,
    );
  });

  it("lets demo visitors read the docs", () => {
    expect(rewrittenTo(proxy(request("/docs", "am_demo=1")))).toBe("/en/docs");
  });

  it("lets requests with am_session through without validating it", () => {
    expect(passesThrough(proxy(request("/apps/x", "am_session=anything")))).toBe(true);
  });

  it("treats an empty am_session cookie as anonymous", () => {
    expect(proxy(request("/settings", "am_session=")).status).toBe(307);
  });

  it("does not treat lookalike paths as public", () => {
    expect(proxy(request("/loginx")).status).toBe(307);
    expect(proxy(request("/setupx/abc")).status).toBe(307);
    expect(proxy(request("/docsx")).status).toBe(307);
  });

  it("does not touch the language of the panel", () => {
    const res = proxy(request("/portfolio", "am_session=x; am_lang=ru"));
    expect(passesThrough(res)).toBe(true);
    expect(savedLocale(res)).toBeNull();
  });

  it("excludes static assets via matcher", () => {
    const matcher = new RegExp(`^${config.matcher[0]}$`);
    expect(matcher.test("/apps/x")).toBe(true);
    expect(matcher.test("/_next/static/chunks/app.js")).toBe(false);
    expect(matcher.test("/_next/image")).toBe(false);
    expect(matcher.test("/favicon.ico")).toBe(false);
  });
});

describe("proxy language routing", () => {
  it("serves English at the root without a saved choice", () => {
    const res = proxy(request("/", undefined, "en-US,en;q=0.9"));
    expect(rewrittenTo(res)).toBe("/en");
    expect(savedLocale(res)).toBeNull();
  });

  it("sends a first visit with a Russian browser to /ru", () => {
    const res = proxy(request("/", undefined, "ru-RU,ru;q=0.9,en;q=0.8"));
    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toBe("http://localhost:3000/ru");
  });

  it("follows the saved choice over the browser language", () => {
    const ru = proxy(request("/docs/recipes/curl?tab=1", "am_lang=ru", "en-US"));
    expect(ru.status).toBe(307);
    expect(ru.headers.get("location")).toBe("http://localhost:3000/ru/docs/recipes/curl?tab=1");

    expect(rewrittenTo(proxy(request("/", "am_lang=en", "ru-RU")))).toBe("/en");
  });

  it.each(["/ru", "/ru/docs", "/ru/docs/recipes/curl"])(
    "serves %s as is and saves Russian for a year",
    (path) => {
      const res = proxy(request(path, "am_lang=en"));
      expect(passesThrough(res)).toBe(true);
      expect(savedLocale(res)).toBe("ru");
      expect(res.headers.get("set-cookie")).toMatch(/Max-Age=31536000/i);
    },
  );

  it.each([
    ["/en", "/"],
    ["/en/docs/recipes/curl?tab=1", "/docs/recipes/curl?tab=1"],
  ])("redirects %s to the address without a prefix and saves English", (path, target) => {
    const res = proxy(request(path, "am_lang=ru"));
    expect(res.status).toBe(308);
    expect(res.headers.get("location")).toBe(`http://localhost:3000${target}`);
    expect(savedLocale(res)).toBe("en");
  });

  it("does not treat lookalike prefixes as languages", () => {
    expect(proxy(request("/russia")).status).toBe(307);
    expect(proxy(request("/english")).status).toBe(307);
  });
});
