import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { type OnUrlUpdateFunction, withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AudienceSummary,
  Breakdown,
  IntegrationHealth,
  Platform,
  TimeSeries,
} from "@/lib/api/gen/model";
import { errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { AudienceView } from "./audience-view";

const APP_ID = "0192aaaa-0000-7000-8000-000000000001";

vi.mock("next/navigation", () => ({
  useParams: () => ({ appId: APP_ID }),
  usePathname: () => `/apps/${APP_ID}`,
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

const toasts = vi.hoisted(() => ({ success: vi.fn(), info: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

const summary = (overrides: Partial<AudienceSummary> = {}): AudienceSummary => ({
  from: "2026-10-07",
  to: "2026-10-13",
  active: 41230,
  new: 7905,
  sessions: 198304,
  avg_duration_ms: 252000,
  median_duration_ms: 125000,
  compare: {
    from: "2026-09-30",
    to: "2026-10-06",
    active: 38823,
    new: 8150,
    sessions: 196140,
    avg_duration_ms: 250000,
    median_duration_ms: 120000,
    delta_pct: {
      active: 6.2,
      new: -3,
      sessions: 1.1,
      avg_duration_ms: 0.8,
      median_duration_ms: 4.2,
    },
  },
  ...overrides,
});

const points = (value: number) => [
  { ts: "2026-10-12T00:00:00+03:00", value },
  { ts: "2026-10-13T00:00:00+03:00", value: value + 512 },
];

const series = (params: URLSearchParams, value = 6000): TimeSeries => ({
  metric: (params.get("metric") ?? "active") as TimeSeries["metric"],
  granularity: (params.get("granularity") ?? "day") as TimeSeries["granularity"],
  points: points(value),
  ...(params.get("split") === "platform"
    ? {
        splits: [
          { platform: "ios", points: points(4000) },
          { platform: "android", points: points(2500) },
        ],
      }
    : {}),
});

const breakdown = (params: URLSearchParams): Breakdown => ({
  dimension: (params.get("dimension") ?? "app_version") as Breakdown["dimension"],
  metric: "active",
  total: 41230,
  rows:
    params.get("dimension") === "platform"
      ? [
          { value: "ios", count: 24001, share: 0.5821 },
          { value: "android", count: 12330, share: 0.299 },
          { value: "web", count: 4899, share: 0.1189 },
        ]
      : [
          { value: "2.1.1", count: 24001, share: 0.5821 },
          { value: "2.1.0", count: 12330, share: 0.299 },
        ],
});

const health = (firstEventAt: string | null): IntegrationHealth => ({
  hours: 24,
  first_event_at: firstEventAt,
  accepted_total: 0,
  rejected_total: 0,
  hourly: [],
  reasons: [],
  versions: [],
});

type Requests = Record<"summary" | "timeseries" | "rolling" | "breakdown", URLSearchParams[]>;

type MockOptions = {
  platforms?: Platform[];
  summary?: AudienceSummary | ((params: URLSearchParams) => Response | AudienceSummary);
  error?: { report: keyof Requests; status: number; code: string };
  firstEventAt?: string | null;
};

function mockApi(opts: MockOptions = {}): Requests {
  const requests: Requests = { summary: [], timeseries: [], rolling: [], breakdown: [] };
  const handle =
    <T,>(report: keyof Requests, body: (params: URLSearchParams) => T | Response) =>
    ({ request }: { request: Request }) => {
      const params = new URL(request.url).searchParams;
      requests[report].push(params);
      if (opts.error && (opts.error.report === report || opts.error.code === "range_too_large")) {
        return HttpResponse.json(errorBody(opts.error.code), { status: opts.error.status });
      }
      const result = body(params);
      return result instanceof Response ? result : HttpResponse.json(result as object);
    };
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(
        makeApp({ id: APP_ID, timezone: "Europe/Moscow", platforms: opts.platforms ?? ["ios"] }),
      ),
    ),
    http.get(
      `*/v1/apps/${APP_ID}/audience/summary`,
      handle("summary", (params) => {
        const s =
          opts.summary ?? summary({ from: params.get("from") ?? "", to: params.get("to") ?? "" });
        return typeof s === "function" ? s(params) : s;
      }),
    ),
    http.get(
      `*/v1/apps/${APP_ID}/audience/timeseries`,
      handle("timeseries", (p) => series(p)),
    ),
    http.get(
      `*/v1/apps/${APP_ID}/audience/rolling-active`,
      handle("rolling", () => ({
        points: [{ day: "2026-10-13", dau: 6512, wau: 41230, mau: 98411 }],
      })),
    ),
    http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, handle("breakdown", breakdown)),
    http.get(`*/v1/apps/${APP_ID}/integration/health`, () =>
      HttpResponse.json(
        health(opts.firstEventAt === undefined ? "2026-09-01T10:00:00Z" : opts.firstEventAt),
      ),
    ),
  );
  return requests;
}

function renderView(searchParams = "", onUrlUpdate?: OnUrlUpdateFunction) {
  return renderWithClient(<AudienceView />, {
    wrapper: withNuqsTestingAdapter({
      searchParams,
      hasMemory: true,
      ...(onUrlUpdate ? { onUrlUpdate } : {}),
    }),
  });
}

const kpis = () => screen.findByRole("region", { name: "Ключевые метрики" });
const lastUrl = (fn: ReturnType<typeof vi.fn<OnUrlUpdateFunction>>) =>
  fn.mock.lastCall?.[0].queryString;

beforeEach(() => {
  // 2026-10-13 01:30 по Москве.
  vi.useFakeTimers({ now: new Date("2026-10-12T22:30:00Z"), shouldAdvanceTime: true });
  for (const fn of Object.values(toasts)) fn.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("AudienceView", () => {
  it("loads every report for the period and filters from the URL", async () => {
    const requests = mockApi();
    renderView("?period=30d&platform=ios");

    const region = await kpis();
    await waitFor(() =>
      expect(within(region).getByRole("button", { name: /^Активные/ })).toHaveTextContent(
        /41\s230/,
      ),
    );
    expect(
      within(region).getByRole("img", { name: /^рост на 6,2\s% к предыдущему периоду$/ }),
    ).toBeInTheDocument();

    const s = requests.summary[0];
    expect(s?.get("from")).toBe("2026-09-14");
    expect(s?.get("to")).toBe("2026-10-13");
    expect(s?.getAll("platform")).toEqual(["ios"]);
    expect(s?.get("environment")).toBe("production");
    expect(s?.get("unit")).toBe("persons");

    expect(
      await screen.findByRole("img", { name: /^График активных пользователей по дням/ }),
    ).toBeInTheDocument();
    expect(await screen.findByRole("img", { name: /^График DAU, WAU и MAU/ })).toBeInTheDocument();
    expect(
      await screen.findByRole("table", { name: "Разбивка: Версия приложения" }),
    ).toBeInTheDocument();
    expect(
      within(await screen.findByRole("radiogroup", { name: "Платформа" })).getByRole("radio", {
        name: "iOS",
      }),
    ).toHaveAttribute("aria-checked", "true");
    expect(screen.queryByText("Платформа: ios")).not.toBeInTheDocument();

    // Спарклайны KPI — дневные ряды каждой метрики, график — текущая метрика.
    await waitFor(() =>
      expect(new Set(requests.timeseries.map((p) => p.get("metric")))).toEqual(
        new Set(["active", "new", "sessions", "avg_duration_ms"]),
      ),
    );
    expect(requests.timeseries.every((p) => p.get("granularity") === "day")).toBe(true);
    expect(requests.breakdown.map((p) => p.get("dimension"))).toContain("app_version");
    expect(requests.rolling[0]?.get("from")).toBe("2026-09-14");
  });

  it("switches reports between platforms", async () => {
    const requests = mockApi({ platforms: ["ios", "android"] });
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderView("?period=30d&country=RU", onUrlUpdate);
    await kpis();

    // Опции — явные платформы и наблюдаемые за период (без фильтров отчёта).
    const switcher = await screen.findByRole("radiogroup", { name: "Платформа" });
    await waitFor(() =>
      expect(
        within(switcher)
          .getAllByRole("radio")
          .map((r) => r.textContent),
      ).toEqual(["Все", "iOS", "Android", "Веб"]),
    );
    const observed = requests.breakdown.find(
      (p) => p.get("dimension") === "platform" && p.get("limit") === "50",
    );
    expect(observed?.get("from")).toBe("2026-09-14");
    expect(observed?.has("country")).toBe(false);

    await user.click(within(switcher).getByRole("radio", { name: "Android" }));
    await waitFor(() =>
      expect(lastUrl(onUrlUpdate)).toBe("?period=30d&country=RU&platform=android"),
    );
    await waitFor(() =>
      expect(requests.summary.some((p) => p.getAll("platform").join() === "android")).toBe(true),
    );
    expect(requests.timeseries.some((p) => p.getAll("platform").join() === "android")).toBe(true);

    // Фильтры отчёта без платформы: «Сбросить фильтры» оставляет выбранную платформу.
    await user.click(screen.getByRole("button", { name: "Сбросить фильтры" }));
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toBe("?period=30d&platform=android"));

    await user.click(within(switcher).getByRole("radio", { name: "Все" }));
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toBe("?period=30d"));
  });

  it("switches the chart to platforms", async () => {
    const requests = mockApi({ platforms: ["ios", "android"] });
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderView("?period=30d&compare=1", onUrlUpdate);
    await kpis();

    const mode = await screen.findByRole("radiogroup", { name: "Режим графика" });
    expect(within(mode).getByRole("radio", { name: "Итог" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await user.click(within(mode).getByRole("radio", { name: "По платформам" }));
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toBe("?period=30d&compare=1&split=platform"));
    await waitFor(() =>
      expect(
        requests.timeseries.some(
          (p) =>
            p.get("split") === "platform" &&
            p.get("compare") === "none" &&
            p.get("metric") === "active" &&
            p.get("granularity") === "day",
        ),
      ).toBe(true),
    );
    // Спарклайны KPI остаются итоговыми рядами: со split запрашивается только график.
    expect(
      requests.timeseries
        .filter((p) => p.get("split") === "platform")
        .every((p) => p.get("metric") === "active"),
    ).toBe(true);

    const legend = await screen.findByRole("list", { name: "Легенда графика" });
    expect(
      within(legend)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual(["iOS", "Android"]);

    // Сравнение в режиме по платформам недоступно, его значение в URL сохраняется.
    const compare = screen.getByRole("switch", { name: "Сравнить с предыдущим периодом" });
    expect(compare).toHaveAttribute("aria-disabled", "true");
    expect(compare).not.toBeChecked();
    expect(compare).toHaveAccessibleDescription("Сравнение недоступно в режиме по платформам");
    await user.click(compare);
    expect(lastUrl(onUrlUpdate)).toBe("?period=30d&compare=1&split=platform");

    await user.click(within(mode).getByRole("radio", { name: "Итог" }));
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toBe("?period=30d&compare=1"));
    expect(screen.getByRole("switch", { name: "Сравнить с предыдущим периодом" })).toBeChecked();
  });

  it("offers the platform mode only for all of two or more platforms", async () => {
    const requests = mockApi({ platforms: ["ios", "android"] });
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderView("?split=platform", onUrlUpdate);
    await kpis();
    await screen.findByRole("list", { name: "Легенда графика" });

    // Выбор платформы выключает режим по платформам.
    await user.click(
      within(await screen.findByRole("radiogroup", { name: "Платформа" })).getByRole("radio", {
        name: "iOS",
      }),
    );
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toBe("?platform=ios"));
    expect(screen.queryByRole("radiogroup", { name: "Режим графика" })).not.toBeInTheDocument();
    await waitFor(() =>
      expect(
        requests.timeseries.some(
          (p) => p.getAll("platform").join() === "ios" && p.get("metric") === "active",
        ),
      ).toBe(true),
    );
    expect(
      requests.timeseries
        .filter((p) => p.getAll("platform").join() === "ios")
        .map((p) => p.get("split")),
    ).not.toContain("platform");
  });

  it("ignores the platform mode for an app with one platform", async () => {
    const requests = mockApi({ platforms: ["ios"] });
    server.use(
      http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, () =>
        HttpResponse.json({
          dimension: "platform",
          metric: "active",
          total: 41230,
          rows: [{ value: "ios", count: 41230, share: 1 }],
        } satisfies Breakdown),
      ),
    );
    renderView("?split=platform");
    await kpis();

    expect(
      await screen.findByRole("img", { name: /^График активных пользователей по дням, / }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: "Режим графика" })).not.toBeInTheDocument();
    expect(
      screen.getByRole("switch", { name: "Сравнить с предыдущим периодом" }),
    ).not.toHaveAttribute("aria-disabled");
    expect(requests.timeseries.map((p) => p.get("split"))).not.toContain("platform");
  });

  it("replaces the platform from a breakdown row", async () => {
    mockApi({ platforms: ["ios", "android"] });
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderView("?dim=platform&platform=ios", onUrlUpdate);
    await kpis();

    await user.click(await screen.findByRole("button", { name: "Фильтр: Платформа android" }));
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toBe("?dim=platform&platform=android"));
  });

  it("puts period, metric, comparison and filters into the URL", async () => {
    const requests = mockApi();
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderView("", onUrlUpdate);
    await kpis();

    await user.click(screen.getByRole("radio", { name: "90 дней" }));
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toBe("?period=90d"));
    expect(onUrlUpdate.mock.lastCall?.[0].options.history).toBe("push");
    await waitFor(() =>
      expect(requests.summary.some((p) => p.get("from") === "2026-07-16")).toBe(true),
    );

    await user.click(await screen.findByRole("button", { name: /^Сессии/ }));
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toBe("?period=90d&metric=sessions"));

    await user.click(screen.getByRole("switch", { name: "Сравнить с предыдущим периодом" }));
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toContain("compare=1"));
    await waitFor(() =>
      expect(
        requests.timeseries.some(
          (p) => p.get("metric") === "sessions" && p.get("compare") === "previous_period",
        ),
      ).toBe(true),
    );

    await user.click(
      await screen.findByRole("button", { name: "Фильтр: Версия приложения 2.1.0" }),
    );
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toContain("app_version=2.1.0"));
    await waitFor(() =>
      expect(requests.summary.some((p) => p.get("app_version") === "2.1.0")).toBe(true),
    );
  });

  it("replaces a granularity the new period does not allow", async () => {
    mockApi();
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderView("?g=hour", onUrlUpdate);
    await kpis();
    expect(screen.getByRole("radio", { name: "час" })).toHaveAttribute("aria-checked", "true");

    await user.click(screen.getByRole("radio", { name: "30 дней" }));
    await waitFor(() => expect(lastUrl(onUrlUpdate)).toBe("?period=30d"));
    expect(toasts.info).toHaveBeenCalledWith("Детализация изменена на «день»");
    expect(screen.getByRole("radio", { name: "день" })).toHaveAttribute("aria-checked", "true");
  });

  it("copies the Read API request", async () => {
    mockApi();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const writeText = vi.spyOn(navigator.clipboard, "writeText");
    renderView("?period=30d&platform=ios");
    await kpis();

    await user.click(screen.getByRole("button", { name: "Скопировать как запрос к Read API" }));
    expect(writeText).toHaveBeenCalledWith(
      `curl -H "Authorization: Bearer $APPMETRICA_SK" "https://api.appmetrica.online/v1/apps/${APP_ID}/audience/summary?from=2026-09-14&to=2026-10-13&platform=ios&environment=production&unit=persons"`,
    );
    expect(toasts.success).toHaveBeenCalledWith("Запрос скопирован", expect.anything());
  });

  it("shows onboarding when the app has never sent events", async () => {
    mockApi({
      summary: summary({
        active: 0,
        new: 0,
        sessions: 0,
        avg_duration_ms: null,
        median_duration_ms: null,
      }),
      firstEventAt: null,
    });
    renderView();

    expect(await screen.findByRole("heading", { name: "Событий ещё нет" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Открыть интеграцию" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/integration`,
    );
    expect(screen.queryByRole("region", { name: "Ключевые метрики" })).not.toBeInTheDocument();
  });

  it("keeps zero KPIs when the period is empty but the app has events", async () => {
    mockApi({
      summary: summary({
        active: 0,
        new: 0,
        sessions: 0,
        avg_duration_ms: null,
        median_duration_ms: null,
      }),
    });
    renderView();

    const region = await kpis();
    await waitFor(() =>
      expect(within(region).getByRole("button", { name: /^Активные/ })).toHaveTextContent("0"),
    );
    expect(screen.queryByRole("heading", { name: "Событий ещё нет" })).not.toBeInTheDocument();
  });

  it("shows an error only in the failed card", async () => {
    mockApi({ error: { report: "summary", status: 503, code: "storage_unavailable" } });
    renderView();

    const region = await kpis();
    expect(
      await within(region).findByText(
        "Хранилище данных временно недоступно. Повторим автоматически.",
      ),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("img", { name: /^График активных пользователей/ }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("table", { name: "Разбивка: Версия приложения" }),
    ).toBeInTheDocument();
  });

  it("shows range_too_large under the period picker", async () => {
    mockApi({ error: { report: "summary", status: 400, code: "range_too_large" } });
    renderView("?from=2024-01-01&to=2026-10-12");

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(
      "Слишком большой период: максимум 366 дней, для детализации по часам — 7 дней.",
    );
    await waitFor(() =>
      expect(
        screen.getAllByText(
          "Слишком большой период: максимум 366 дней, для детализации по часам — 7 дней.",
        ),
      ).toHaveLength(1),
    );
  });

  it("warns that recent days may still change", async () => {
    mockApi();
    const { unmount } = renderView();
    expect(
      await screen.findByText(
        "Данные за последние 8 дней могут дополниться событиями из офлайн-очередей (окно 7 дней).",
      ),
    ).toBeInTheDocument();
    unmount();

    renderView("?from=2026-08-01&to=2026-08-31");
    await kpis();
    expect(screen.queryByText(/могут дополниться/)).not.toBeInTheDocument();
  });
});
