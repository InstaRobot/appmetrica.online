import { HttpResponse, http } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw";
import { ApiError, apiFetch } from "./fetcher";

const demoFetch = vi.hoisted(() => vi.fn<(url: string, init?: RequestInit) => Promise<Response>>());
vi.mock("@/lib/demo/handler", () => ({ demoFetch }));

type Envelope<T> = { data: T; status: number; headers: Headers };

const originalLocation = Object.getOwnPropertyDescriptor(window, "location");

function stubLocation(path: string) {
  const url = new URL(path, "http://localhost:3000");
  const assign = vi.fn<(url: string) => void>();
  Object.defineProperty(window, "location", {
    configurable: true,
    value: { pathname: url.pathname, search: url.search, href: url.href, assign },
  });
  return assign;
}

describe("apiFetch in the browser", () => {
  let assign: ReturnType<typeof stubLocation>;

  beforeEach(() => {
    assign = stubLocation("/apps/1?from=2026-10-01");
  });

  afterEach(() => {
    if (originalLocation) Object.defineProperty(window, "location", originalLocation);
  });

  it("redirects to login on 401", async () => {
    server.use(
      http.get("*/api/v1/portfolio", () =>
        HttpResponse.json(
          { error: { code: "unauthenticated", message: "Требуется вход", request_id: "01JREQ" } },
          { status: 401 },
        ),
      ),
    );

    const promise = apiFetch("/v1/portfolio");

    await expect(promise).rejects.toBeInstanceOf(ApiError);
    await expect(promise).rejects.toMatchObject({
      status: 401,
      code: "unauthenticated",
      requestId: "01JREQ",
    });
    expect(assign).toHaveBeenCalledExactlyOnceWith("/login?next=%2Fapps%2F1%3Ffrom%3D2026-10-01");
  });

  it("returns orval envelope with data, status and headers", async () => {
    server.use(
      http.get("*/api/v1/apps", () =>
        HttpResponse.json({ apps: [] }, { status: 200, headers: { "X-Request-Id": "01JOK" } }),
      ),
    );

    const res = await apiFetch<Envelope<{ apps: unknown[] }>>("/v1/apps");

    expect(res.status).toBe(200);
    expect(res.data).toEqual({ apps: [] });
    expect(res.headers.get("x-request-id")).toBe("01JOK");
    expect(assign).not.toHaveBeenCalled();
  });

  it("calls same-origin /api with cookies and no CSRF header on GET", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    let csrf: string | null = "unset";
    server.use(
      http.get("*/api/v1/apps", ({ request }) => {
        csrf = request.headers.get("x-requested-with");
        return HttpResponse.json({ apps: [] });
      }),
    );

    await apiFetch("/v1/apps", { method: "GET" });

    const [url, init] = fetchSpy.mock.calls[0] ?? [];
    expect(url).toBe("/api/v1/apps");
    expect(init?.credentials).toBe("include");
    expect(csrf).toBeNull();
  });

  it.each(["POST", "PATCH", "PUT", "DELETE"])("sends X-Requested-With on %s", async (method) => {
    let csrf: string | null = null;
    server.use(
      http.all("*/api/admin/v1/apps", ({ request }) => {
        csrf = request.headers.get("x-requested-with");
        return new HttpResponse(null, { status: 204 });
      }),
    );

    const res = await apiFetch<Envelope<undefined>>("/admin/v1/apps", { method });

    expect(csrf).toBe("appmetrica-web");
    expect(res.status).toBe(204);
    expect(res.data).toBeUndefined();
  });

  it("keeps caller headers on mutations", async () => {
    let contentType: string | null = null;
    server.use(
      http.post("*/api/admin/v1/apps", ({ request }) => {
        contentType = request.headers.get("content-type");
        return HttpResponse.json({ id: "1" }, { status: 201 });
      }),
    );

    await apiFetch("/admin/v1/apps", {
      method: "post",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: "Notes" }),
    });

    expect(contentType).toBe("application/json");
  });
});

describe("apiFetch errors", () => {
  it("parses the Error envelope without redirecting on 404", async () => {
    const assign = stubLocation("/apps/x");
    server.use(
      http.get("*/api/v1/apps/x", () =>
        HttpResponse.json(
          {
            error: {
              code: "app_not_found",
              message: "Приложение не найдено",
              request_id: "01JNF",
              details: { app_id: "x" },
            },
          },
          { status: 404 },
        ),
      ),
    );

    const err = await apiFetch("/v1/apps/x").catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      status: 404,
      code: "app_not_found",
      message: "Приложение не найдено",
      requestId: "01JNF",
      details: { app_id: "x" },
    });
    expect(assign).not.toHaveBeenCalled();
    if (originalLocation) Object.defineProperty(window, "location", originalLocation);
  });

  it("falls back to http_<status> when the body is not an Error envelope", async () => {
    server.use(
      http.get("*/api/v1/portfolio", () => new HttpResponse("Bad Gateway", { status: 502 })),
    );

    await expect(apiFetch("/v1/portfolio")).rejects.toMatchObject({
      status: 502,
      code: "http_502",
      requestId: "",
    });
  });

  it("maps network failures to ApiError(0, network_error)", async () => {
    server.use(http.get("*/api/v1/portfolio", () => HttpResponse.error()));

    await expect(apiFetch("/v1/portfolio")).rejects.toMatchObject({
      status: 0,
      code: "network_error",
    });
  });

  it("propagates AbortError from the caller signal", async () => {
    server.use(http.get("*/api/v1/portfolio", () => HttpResponse.json({})));
    const controller = new AbortController();
    controller.abort();

    await expect(apiFetch("/v1/portfolio", { signal: controller.signal })).rejects.toMatchObject({
      name: "AbortError",
    });
  });
});

describe("apiFetch in demo mode", () => {
  beforeEach(() => {
    // biome-ignore lint/suspicious/noDocumentCookie: тест эмулирует cookie демо-режима
    document.cookie = "am_demo=1; path=/";
    demoFetch.mockReset();
  });

  afterEach(() => {
    // biome-ignore lint/suspicious/noDocumentCookie: тест эмулирует cookie демо-режима
    document.cookie = "am_demo=; max-age=0; path=/";
    if (originalLocation) Object.defineProperty(window, "location", originalLocation);
  });

  it("answers from the static demo dataset without touching the network", async () => {
    let hit = false;
    server.use(
      http.get("*/api/v1/portfolio", () => {
        hit = true;
        return HttpResponse.json({});
      }),
    );
    demoFetch.mockResolvedValue(Response.json({ date: "2026-10-13", rows: [] }));

    const res = await apiFetch<Envelope<unknown>>("/v1/portfolio?date=2026-10-13");

    expect(res).toMatchObject({ status: 200, data: { date: "2026-10-13", rows: [] } });
    expect(demoFetch).toHaveBeenCalledWith(
      "/v1/portfolio?date=2026-10-13",
      expect.objectContaining({ method: "GET" }),
    );
    expect(hit).toBe(false);
  });

  it("rejects changes with demo_read_only and never sends the visitor to login", async () => {
    const assign = stubLocation("/apps/1/settings");
    demoFetch.mockResolvedValueOnce(
      Response.json(
        {
          error: {
            code: "demo_read_only",
            message: "Демо-режим: изменения недоступны",
            request_id: "demo",
          },
        },
        { status: 403 },
      ),
    );

    await expect(apiFetch("/admin/v1/apps", { method: "POST", body: "{}" })).rejects.toMatchObject({
      status: 403,
      code: "demo_read_only",
    });

    demoFetch.mockResolvedValueOnce(
      Response.json(
        { error: { code: "unauthenticated", message: "x", request_id: "demo" } },
        { status: 401 },
      ),
    );
    await expect(apiFetch("/v1/apps")).rejects.toMatchObject({ status: 401 });
    expect(assign).not.toHaveBeenCalled();
  });
});
