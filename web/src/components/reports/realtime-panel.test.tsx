import { focusManager, QueryClientProvider } from "@tanstack/react-query";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { type OnUrlUpdateFunction, withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Breakdown, Platform, Realtime, RecentEvent } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { renderWithLocale } from "@/test/rtl";
import { RealtimePanel } from "./realtime-panel";
import { TOUCH_BUTTON } from "./responsive";

const APP_ID = "0192aaaa-0000-7000-8000-000000000007";
// 2026-10-13 14:32:05 по Москве.
const NOW = new Date("2026-10-13T11:32:05Z");

function event(n: number, overrides: Partial<RecentEvent> = {}): RecentEvent {
  const id = String(n).padStart(12, "0");
  return {
    insert_id: `0192bbbb-0000-7000-8000-${id}`,
    event_time: new Date(NOW.getTime() - n * 1000).toISOString(),
    received_at: new Date(NOW.getTime() - n * 1000).toISOString(),
    name: "session_start",
    environment: "production",
    platform: "ios",
    app_version: "2.1.1",
    os_name: "iOS",
    os_version: "26.1",
    device_model: "iPhone17,1",
    country: "RU",
    city: "Москва",
    install_id: `5d8e0000-0000-4000-8000-${id}`,
    person_key: `i:5d8e0000-0000-4000-8000-${id}`,
    ...overrides,
  };
}

function realtime(poll: number, overrides: Partial<Realtime> = {}): Realtime {
  const generated = new Date(NOW.getTime() + poll * 5000);
  return {
    active_now: 1284 + poll,
    per_minute: Array.from({ length: 30 }, (_, i) => ({
      minute: new Date(Date.UTC(2026, 9, 13, 11, 3 + i)).toISOString(),
      sessions_started: i % 5,
    })),
    recent_events: [event(1), event(2, { name: "session_end", platform: "android" })],
    generated_at: generated.toISOString(),
    ...overrides,
  };
}

type MockOptions = {
  platforms?: Platform[];
  /** Ответ на n-й (с нуля) запрос; по умолчанию — данные. */
  respond?: (n: number) => Realtime | { status: number; code: string };
};

// Наблюдаемые платформы для переключателя.
const platformBreakdown: Breakdown = {
  dimension: "platform",
  metric: "active",
  total: 100,
  rows: [
    { value: "ios", count: 70, share: 0.7 },
    { value: "android", count: 30, share: 0.3 },
  ],
};

function mockApi(opts: MockOptions = {}) {
  const requests: URLSearchParams[] = [];
  const breakdowns: URLSearchParams[] = [];
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(
        makeApp({ id: APP_ID, timezone: "Europe/Moscow", platforms: opts.platforms ?? ["ios"] }),
      ),
    ),
    http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, ({ request }) => {
      breakdowns.push(new URL(request.url).searchParams);
      return HttpResponse.json(platformBreakdown);
    }),
    http.get(`*/v1/apps/${APP_ID}/realtime`, ({ request }) => {
      const n = requests.length;
      requests.push(new URL(request.url).searchParams);
      const body = opts.respond ? opts.respond(n) : realtime(n);
      if ("code" in body) {
        return HttpResponse.json(errorBody(body.code), { status: body.status });
      }
      return HttpResponse.json(body);
    }),
  );
  return Object.assign(requests, { breakdowns });
}

function renderPanel(searchParams = "", onUrlUpdate?: OnUrlUpdateFunction) {
  return renderWithClient(<RealtimePanel appId={APP_ID} />, {
    wrapper: withNuqsTestingAdapter({
      searchParams,
      hasMemory: true,
      ...(onUrlUpdate ? { onUrlUpdate } : {}),
    }),
  });
}

const counter = () => screen.findByRole("region", { name: "Активны сейчас" });
const value = (region: HTMLElement) =>
  region.querySelector("[data-value]")?.getAttribute("data-value");

async function poll(ms = 5000) {
  await act(() => vi.advanceTimersByTimeAsync(ms));
}

beforeEach(() => {
  vi.useFakeTimers({ now: NOW, shouldAdvanceTime: true });
});

afterEach(() => {
  focusManager.setFocused(undefined);
  vi.useRealTimers();
});

describe("RealtimePanel", () => {
  it("pauses polling", async () => {
    const requests = mockApi();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPanel();

    const region = await counter();
    expect(value(region)).toBe("1284");
    expect(requests).toHaveLength(1);

    await poll();
    await waitFor(() => expect(requests).toHaveLength(2));
    await waitFor(() => expect(value(region)).toBe("1285"));

    const pause = screen.getByRole("button", { name: "Пауза" });
    expect(pause).toHaveAttribute("aria-pressed", "false");
    await user.click(pause);
    const resume = screen.getByRole("button", { name: "Продолжить" });
    expect(resume).toHaveAttribute("aria-pressed", "true");
    expect(within(region).getByText("на паузе")).toBeInTheDocument();

    const paused = requests.length;
    await poll(15_000);
    expect(requests).toHaveLength(paused);

    await user.click(resume);
    await waitFor(() => expect(requests).toHaveLength(paused + 1));
    expect(screen.getByRole("button", { name: "Пауза" })).toHaveAttribute("aria-pressed", "false");
    await poll();
    await waitFor(() => expect(requests).toHaveLength(paused + 2));
  });

  it("renders the counter, per-minute chart and event feed for the URL environment", async () => {
    const requests = mockApi();
    renderPanel("?env=development");

    const region = await counter();
    expect(within(region).getByText(/^1\s284$/)).toBeInTheDocument();
    expect(within(region).getByText("за последние 30 мин")).toBeInTheDocument();
    expect(within(region).getByText("обновлено 14:32:05")).toBeInTheDocument();
    expect(requests[0]?.get("environment")).toBe("development");
    expect(screen.getByRole("radio", { name: "development" })).toHaveAttribute(
      "aria-checked",
      "true",
    );

    expect(
      screen.getByRole("img", { name: /^Старты сессий по минутам, 14:03–14:32/ }),
    ).toBeInTheDocument();
    const feed = screen.getByRole("table", { name: "Последние события" });
    const rows = within(feed).getAllByRole("row");
    expect(rows).toHaveLength(3);
    expect(within(rows[1] as HTMLElement).getByText("session_start")).toBeInTheDocument();
    expect(within(rows[2] as HTMLElement).getByText("Android")).toBeInTheDocument();
  });

  // jsdom не считает CSS: телефонная вёрстка проверяется по брейкпоинт-классам.
  it("stacks the counter over the chart and stretches the switches on a phone", async () => {
    mockApi();
    renderPanel();

    const region = await counter();
    expect(region.parentElement).toHaveClass("grid-cols-1", "md:grid-cols-[320px_1fr]");
    expect(screen.getByRole("button", { name: "Пауза" })).toHaveClass(TOUCH_BUTTON);
    expect(screen.getByRole("radiogroup", { name: "Окружение" }).parentElement).toHaveClass(
      "w-full",
      "md:ml-auto",
      "md:w-auto",
    );
  });

  it("changes environment in the URL and refetches", async () => {
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const requests = mockApi();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPanel("", onUrlUpdate);
    await counter();
    expect(requests[0]?.get("environment")).toBe("production");

    await user.click(screen.getByRole("radio", { name: "development" }));
    expect(onUrlUpdate.mock.lastCall?.[0].queryString).toBe("?env=development");
    await waitFor(() => expect(requests.at(-1)?.get("environment")).toBe("development"));
  });

  it("narrows realtime to the platform from the switcher", async () => {
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const requests = mockApi({ platforms: ["ios", "android"] });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPanel("?platform=android&env=development", onUrlUpdate);
    await counter();
    expect(requests[0]?.getAll("platform")).toEqual(["android"]);

    // Наблюдаемые платформы — за сегодня в TZ приложения и в выбранном окружении.
    await waitFor(() => expect(requests.breakdowns).toHaveLength(1));
    const observed = requests.breakdowns[0];
    expect(observed?.get("dimension")).toBe("platform");
    expect(observed?.get("from")).toBe("2026-10-13");
    expect(observed?.get("to")).toBe("2026-10-13");
    expect(observed?.get("environment")).toBe("development");
    expect(observed?.has("platform")).toBe(false);

    const switcher = screen.getByRole("radiogroup", { name: "Платформа" });
    expect(within(switcher).getByRole("radio", { name: "Android" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await user.click(within(switcher).getByRole("radio", { name: "iOS" }));
    expect(onUrlUpdate.mock.lastCall?.[0].queryString).toBe("?env=development&platform=ios");
    await waitFor(() => expect(requests.at(-1)?.getAll("platform")).toEqual(["ios"]));

    await user.click(within(switcher).getByRole("radio", { name: "Все" }));
    expect(onUrlUpdate.mock.lastCall?.[0].queryString).toBe("?env=development");
    await waitFor(() => expect(requests.at(-1)?.has("platform")).toBe(false));
  });

  it("hides the switcher for a single-platform app", async () => {
    const requests = mockApi({ platforms: ["ios"] });
    server.use(
      http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, () =>
        HttpResponse.json({ ...platformBreakdown, rows: [platformBreakdown.rows[0]] }),
      ),
    );
    renderPanel();
    await counter();
    await waitFor(() => expect(requests.length).toBeGreaterThan(0));
    expect(screen.queryByRole("radiogroup", { name: "Платформа" })).not.toBeInTheDocument();
  });

  it("shows skeletons until the first response", async () => {
    mockApi();
    renderPanel();

    expect(screen.getAllByRole("status", { name: "Загрузка…" }).length).toBeGreaterThan(0);
    await counter();
    expect(screen.queryByRole("status", { name: "Загрузка…" })).not.toBeInTheDocument();
  });

  it("shows the empty state when nothing happened in 30 minutes", async () => {
    mockApi({
      respond: (n) =>
        realtime(n, {
          active_now: 0,
          recent_events: [],
          per_minute: realtime(n).per_minute.map((p) => ({ ...p, sessions_started: 0 })),
        }),
    });
    renderPanel();

    const region = await counter();
    expect(value(region)).toBe("0");
    expect(screen.getByText("За последние 30 минут событий нет")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Отправьте тестовое событие из сниппета на вкладке «Интеграция» — оно появится здесь в течение 10 секунд.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Последние события" })).not.toBeInTheDocument();
    expect(screen.getByRole("img", { name: /^Старты сессий по минутам/ })).toBeInTheDocument();
  });

  it("keeps the last data and warns after three failed polls in a row", async () => {
    const requests = mockApi({
      respond: (n) =>
        (n >= 1 && n <= 3) || n === 5 ? { status: 500, code: "internal" } : realtime(n),
    });
    renderPanel();
    const region = await counter();

    await poll();
    await poll();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    await poll();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Не удаётся обновить данные. Показаны данные на 14:32:05.");
    expect(value(region)).toBe("1284");
    expect(within(region).getByText("нет связи")).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Последние события" })).toBeInTheDocument();

    // Опрос продолжается: следующий успешный ответ снимает предупреждение.
    await poll();
    await waitFor(() => expect(value(region)).toBe("1288"));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(within(region).getByText("обновлено 14:32:25")).toBeInTheDocument();

    // Счёт ошибок начинается заново после успеха: одна неудача — ещё не предупреждение.
    await poll();
    await waitFor(() => expect(requests).toHaveLength(6));
    await poll(1000);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("shows the error with retry when the first request fails", async () => {
    const requests = mockApi({
      respond: (n) => (n === 0 ? { status: 503, code: "storage_unavailable" } : realtime(n)),
    });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPanel();

    await user.click(await screen.findByRole("button", { name: "Повторить" }));
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(value(await counter())).toBe("1285");
  });

  it("stops polling while the tab is hidden", async () => {
    const requests = mockApi();
    renderPanel();
    await counter();

    act(() => focusManager.setFocused(false));
    await poll(15_000);
    expect(requests).toHaveLength(1);

    act(() => focusManager.setFocused(true));
    await poll();
    await waitFor(() => expect(requests.length).toBeGreaterThan(1));
  });
});

function renderPanelInEnglish() {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return renderWithLocale(
    <QueryClientProvider client={client}>
      <RealtimePanel appId={APP_ID} />
    </QueryClientProvider>,
    "en",
    { wrapper: withNuqsTestingAdapter({ searchParams: "", hasMemory: true }) },
  );
}

describe("RealtimePanel in English", () => {
  it("labels the counter, chart, feed and pause in English", async () => {
    mockApi();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPanelInEnglish();

    const region = await screen.findByRole("region", { name: "Active now" });
    expect(within(region).getByText("1,284")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Session starts per minute" })).toBeInTheDocument();
    expect(
      await screen.findByRole("img", { name: /^Session starts per minute, 14:03–14:32/ }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Recent events" })).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Recent events" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Pause" }));
    expect(screen.getByRole("button", { name: "Resume" })).toHaveAttribute("aria-pressed", "true");
    expect(within(region).getByText("paused")).toBeInTheDocument();
  });

  it("shows the empty feed in English", async () => {
    mockApi({ respond: (n) => realtime(n, { recent_events: [] }) });
    renderPanelInEnglish();

    expect(await screen.findByText("No events in the last 30 minutes")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Send a test event from a snippet on the Integration tab — it will show up here within 10 seconds.",
      ),
    ).toBeInTheDocument();
  });

  it("warns in English after three failed polls in a row", async () => {
    mockApi({
      respond: (n) => (n >= 1 && n <= 3 ? { status: 500, code: "internal" } : realtime(n)),
    });
    renderPanelInEnglish();
    await screen.findByRole("region", { name: "Active now" });

    await poll();
    await poll();
    await poll();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Can't refresh the data. Showing data as of 14:32:05.",
    );
  });
});
