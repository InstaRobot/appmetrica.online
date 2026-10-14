import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";
import { config, proxy } from "./proxy";

function request(path: string, cookie?: string) {
  const headers = new Headers();
  if (cookie) headers.set("cookie", cookie);
  return new NextRequest(new URL(path, "http://localhost:3000"), { headers });
}

const passesThrough = (res: Response) => res.headers.get("x-middleware-next") === "1";

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
    expect(passesThrough(proxy(request("/")))).toBe(true);
  });

  it.each(["/login", "/login?next=%2Fapps", "/setup/abc123", "/api/health", "/api/v1/portfolio"])(
    "lets anonymous requests to %s through",
    (path) => {
      expect(passesThrough(proxy(request(path)))).toBe(true);
    },
  );

  it("lets requests with am_session through without validating it", () => {
    expect(passesThrough(proxy(request("/apps/x", "am_session=anything")))).toBe(true);
  });

  it("treats an empty am_session cookie as anonymous", () => {
    expect(proxy(request("/settings", "am_session=")).status).toBe(307);
  });

  it("does not treat lookalike paths as public", () => {
    expect(proxy(request("/loginx")).status).toBe(307);
    expect(proxy(request("/setupx/abc")).status).toBe(307);
  });

  it("excludes static assets via matcher", () => {
    const matcher = new RegExp(`^${config.matcher[0]}$`);
    expect(matcher.test("/apps/x")).toBe(true);
    expect(matcher.test("/_next/static/chunks/app.js")).toBe(false);
    expect(matcher.test("/_next/image")).toBe(false);
    expect(matcher.test("/favicon.ico")).toBe(false);
  });
});
