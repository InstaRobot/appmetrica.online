// @vitest-environment node
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw";

const cookieHeader = vi.hoisted(() => ({ value: "am_session=s3cr3t" }));
const redirect = vi.hoisted(() =>
  vi.fn((url: string) => {
    throw new Error(`NEXT_REDIRECT ${url}`);
  }),
);

const demoFetch = vi.hoisted(() => vi.fn<(url: string, init?: RequestInit) => Promise<Response>>());
vi.mock("@/lib/demo/handler", () => ({ demoFetch }));

vi.mock("next/headers", () => ({
  cookies: async () => ({
    toString: () => cookieHeader.value,
    get: (name: string) => {
      const pair = cookieHeader.value
        .split("; ")
        .map((c) => c.split("="))
        .find(([key]) => key === name);
      return pair ? { name, value: pair[1] ?? "" } : undefined;
    },
  }),
}));
vi.mock("next/navigation", () => ({ redirect }));

const { apiFetch } = await import("./fetcher");

describe("apiFetch on the server", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    cookieHeader.value = "am_session=s3cr3t";
  });

  it("renders demo pages from the static dataset on the server", async () => {
    cookieHeader.value = "am_demo=1";
    demoFetch.mockResolvedValue(Response.json({ items: [] }));

    const res = await apiFetch<{ data: unknown; status: number }>("/v1/apps");

    expect(res).toMatchObject({ status: 200, data: { items: [] } });
    expect(demoFetch).toHaveBeenCalledWith("/v1/apps", expect.objectContaining({ method: "GET" }));
  });

  it("calls API_INTERNAL_URL directly and forwards the session cookie", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://appmetrica-api:8080");
    let seen: { url: string; cookie: string | null } | undefined;
    server.use(
      http.get("http://appmetrica-api:8080/v1/portfolio", ({ request }) => {
        seen = { url: request.url, cookie: request.headers.get("cookie") };
        return HttpResponse.json({ rows: [] });
      }),
    );

    const res = await apiFetch<{ data: unknown; status: number }>("/v1/portfolio?date=2026-10-12");

    expect(res).toMatchObject({ status: 200, data: { rows: [] } });
    expect(seen).toEqual({
      url: "http://appmetrica-api:8080/v1/portfolio?date=2026-10-12",
      cookie: "am_session=s3cr3t",
    });
  });

  it("defaults to http://localhost:8080 without API_INTERNAL_URL", async () => {
    vi.stubEnv("API_INTERNAL_URL", undefined);
    cookieHeader.value = "";
    let cookie: string | null = "unset";
    server.use(
      http.get("http://localhost:8080/health", ({ request }) => {
        cookie = request.headers.get("cookie");
        return HttpResponse.json({ status: "ok" });
      }),
    );

    await expect(apiFetch("/health")).resolves.toMatchObject({ status: 200 });
    expect(cookie).toBeNull();
  });

  it("redirects to /login on 401", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://appmetrica-api:8080");
    server.use(
      http.get("http://appmetrica-api:8080/v1/portfolio", () =>
        HttpResponse.json(
          { error: { code: "unauthenticated", message: "x", request_id: "r" } },
          { status: 401 },
        ),
      ),
    );

    await expect(apiFetch("/v1/portfolio")).rejects.toThrow("NEXT_REDIRECT /login");
    expect(redirect).toHaveBeenCalledWith("/login");
  });
});
