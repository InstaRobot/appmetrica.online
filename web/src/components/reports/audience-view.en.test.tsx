import { QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AudienceView } from "@/app/(dashboard)/apps/[appId]/audience-view";
import type { AudienceSummary, Breakdown, Platform, TimeSeries } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithLocale } from "@/test/rtl";

// Экран S-05 на английском: подписи из словаря en, числа, длительности и даты в формате en-US.
const APP_ID = "0192aaaa-0000-7000-8000-000000000001";

vi.mock("next/navigation", () => ({
  useParams: () => ({ appId: APP_ID }),
  usePathname: () => `/apps/${APP_ID}`,
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

const toasts = vi.hoisted(() => ({ success: vi.fn(), info: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

const summary = (active = 41230): AudienceSummary => ({
  from: "2026-10-07",
  to: "2026-10-13",
  active,
  new: active === 0 ? 0 : 7905,
  sessions: active === 0 ? 0 : 198304,
  avg_duration_ms: active === 0 ? null : 252000,
  median_duration_ms: active === 0 ? null : 125000,
});

const series = (params: URLSearchParams): TimeSeries => {
  const points = [
    { ts: "2026-10-12T00:00:00+03:00", value: 6000 },
    { ts: "2026-10-13T00:00:00+03:00", value: 6512 },
  ];
  return {
    metric: (params.get("metric") ?? "active") as TimeSeries["metric"],
    granularity: (params.get("granularity") ?? "day") as TimeSeries["granularity"],
    points,
    ...(params.get("split") === "platform"
      ? {
          splits: [
            { platform: "ios", points },
            { platform: "android", points },
          ],
        }
      : {}),
  };
};

const breakdown = (params: URLSearchParams): Breakdown => ({
  dimension: (params.get("dimension") ?? "app_version") as Breakdown["dimension"],
  metric: "active",
  total: 41230,
  rows:
    params.get("dimension") === "platform"
      ? [
          { value: "ios", count: 24001, share: 0.5821 },
          { value: "android", count: 17229, share: 0.4179 },
        ]
      : [{ value: "2.1.1", count: 24001, share: 0.5821 }],
});

function mockApi(opts: { platforms?: Platform[]; active?: number } = {}) {
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(
        makeApp({ id: APP_ID, timezone: "Europe/Moscow", platforms: opts.platforms ?? ["ios"] }),
      ),
    ),
    http.get(`*/v1/apps/${APP_ID}/audience/summary`, () => HttpResponse.json(summary(opts.active))),
    http.get(`*/v1/apps/${APP_ID}/audience/timeseries`, ({ request }) =>
      HttpResponse.json(series(new URL(request.url).searchParams)),
    ),
    http.get(`*/v1/apps/${APP_ID}/audience/rolling-active`, () =>
      HttpResponse.json({ points: [{ day: "2026-10-13", dau: 6512, wau: 41230, mau: 98411 }] }),
    ),
    http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, ({ request }) =>
      HttpResponse.json(breakdown(new URL(request.url).searchParams)),
    ),
    http.get(`*/v1/apps/${APP_ID}/integration/health`, () =>
      HttpResponse.json({
        hours: 24,
        first_event_at: null,
        accepted_total: 0,
        rejected_total: 0,
        hourly: [],
        reasons: [],
        versions: [],
      }),
    ),
  );
}

function renderView(searchParams = "") {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return renderWithLocale(
    <QueryClientProvider client={client}>
      <AudienceView />
    </QueryClientProvider>,
    "en",
    { wrapper: withNuqsTestingAdapter({ searchParams, hasMemory: true }) },
  );
}

beforeEach(() => {
  // 2026-10-13 01:30 по Москве.
  vi.useFakeTimers({ now: new Date("2026-10-12T22:30:00Z"), shouldAdvanceTime: true });
  for (const fn of Object.values(toasts)) fn.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("AudienceView in English", () => {
  it("labels the report cards and formats their numbers in English", async () => {
    mockApi();
    renderView();

    const region = await screen.findByRole("region", { name: "Key metrics" });
    await waitFor(() =>
      expect(within(region).getByRole("button", { name: /^Active users/ })).toHaveTextContent(
        "41,230",
      ),
    );
    expect(within(region).getByRole("button", { name: /^Avg\. duration/ })).toHaveTextContent(
      "4m 12smedian 2m 5s",
    );
    expect(
      await screen.findByRole("img", { name: /^Active users chart by day, Oct\s12–13, 2026/ }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("img", { name: /^DAU, WAU and MAU chart, Oct\s13, 2026/ }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Breakdown" })).toBeInTheDocument();
    expect(
      await screen.findByRole("table", { name: "Breakdown: App version" }),
    ).toBeInTheDocument();

    const chartMetric = screen.getByRole("radiogroup", { name: "Chart metric" });
    expect(
      within(chartMetric)
        .getAllByRole("radio")
        .map((r) => r.textContent),
    ).toEqual(["Active users", "New users", "Sessions", "Avg. duration"]);
    expect(
      screen.getByText(
        "Data for the last 8 days may still be updated with events from offline queues (7-day window).",
      ),
    ).toBeInTheDocument();
  });

  it("names the chart modes and explains why comparison is unavailable", async () => {
    mockApi({ platforms: ["ios", "android"] });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderView("?compare=1");

    const mode = await screen.findByRole("radiogroup", { name: "Chart mode" });
    expect(within(mode).getByRole("radio", { name: "Total" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await user.click(within(mode).getByRole("radio", { name: "By platform" }));

    const compare = await screen.findByRole("switch", { name: "Compare with the previous period" });
    await waitFor(() => expect(compare).toHaveAttribute("aria-disabled", "true"));
    expect(compare).toHaveAccessibleDescription(
      "Comparison isn't available in the by-platform mode",
    );
    expect(
      within(await screen.findByRole("radiogroup", { name: "Platform" })).getByRole("radio", {
        name: "All",
      }),
    ).toHaveAttribute("aria-checked", "true");
  });

  it("reports a replaced granularity and a copied request in English", async () => {
    mockApi();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.spyOn(navigator.clipboard, "writeText");
    renderView("?g=hour");
    await screen.findByRole("region", { name: "Key metrics" });

    await user.click(screen.getByRole("radio", { name: "30 days" }));
    await waitFor(() => expect(toasts.info).toHaveBeenCalledWith("Granularity changed to “Day”"));

    await user.click(screen.getByRole("button", { name: "Copy as Read API request" }));
    expect(toasts.success).toHaveBeenCalledWith("Request copied", expect.anything());
  });

  it("shows onboarding in English when the app has never sent events", async () => {
    mockApi({ active: 0 });
    renderView();

    expect(await screen.findByRole("heading", { name: "No events yet" })).toBeInTheDocument();
    expect(
      screen.getByText("Send your first event — snippets are on the Integration tab."),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open integration" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/integration`,
    );
  });
});
