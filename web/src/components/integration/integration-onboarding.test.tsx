import { QueryClientProvider } from "@tanstack/react-query";
import { act, screen, waitFor, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Breakdown, IntegrationHealth, Platform, WriteKey } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { renderWithLocale } from "@/test/rtl";
import { IntegrationPanel } from "./integration-panel";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

const APP_ID = "0192aaaa-0000-7000-8000-000000000010";
// 2026-10-13 14:32:05 по Москве.
const NOW = new Date("2026-10-13T11:32:05Z");
const KEY = "pk_7Hq2x9AbLm3Np4Qr5St6Uv7Wx8Yz9Ab0";

// Тот же QueryClient без повторов, что у renderWithClient, но с английским словарём.
function renderInEnglish(ui: ReactElement) {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return renderWithLocale(ui, "en", {
    wrapper: ({ children }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
}

function health(overrides: Partial<IntegrationHealth> = {}): IntegrationHealth {
  return {
    hours: 24,
    first_event_at: null,
    accepted_total: 0,
    rejected_total: 0,
    hourly: [],
    reasons: [],
    versions: [],
    ...overrides,
  };
}

function writeKey(overrides: Partial<WriteKey> = {}): WriteKey {
  return {
    id: "0192cccc-0000-7000-8000-000000000001",
    app_id: APP_ID,
    key: KEY,
    label: "",
    platform: null,
    created_at: "2026-10-13T11:00:00Z",
    revoked_at: null,
    last_used_at: null,
    ...overrides,
  };
}

type MockOptions = {
  platforms?: [Platform, ...Platform[]];
  keys?: WriteKey[];
  /** Платформы, наблюдавшиеся за 48 ч, по окружениям. */
  observed?: { production?: string[]; development?: string[] };
  /** Ответ на n-й (с нуля) запрос здоровья. */
  health?: (n: number) => IntegrationHealth;
};

function breakdown(values: string[]): Breakdown {
  return {
    dimension: "platform",
    metric: "active",
    total: values.length,
    rows: values.map((value) => ({ value, count: 1, share: 1 / values.length })),
  };
}

function mockApi(opts: MockOptions = {}) {
  const requests: URLSearchParams[] = [];
  const breakdownRequests: URLSearchParams[] = [];
  const platforms: [Platform, ...Platform[]] = opts.platforms ?? ["ios"];
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(
        makeApp({ id: APP_ID, name: "Notes iOS", platform: platforms[0], platforms }),
      ),
    ),
    http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, ({ request }) => {
      const params = new URL(request.url).searchParams;
      breakdownRequests.push(params);
      const env = params.get("environment") === "development" ? "development" : "production";
      return HttpResponse.json(breakdown(opts.observed?.[env] ?? []));
    }),
    http.get(`*/admin/v1/apps/${APP_ID}/write-keys`, () =>
      HttpResponse.json({ items: opts.keys ?? [writeKey()] }),
    ),
    http.get(`*/v1/apps/${APP_ID}/integration/health`, ({ request }) => {
      const n = requests.length;
      requests.push(new URL(request.url).searchParams);
      return HttpResponse.json(opts.health ? opts.health(n) : health());
    }),
  );
  return Object.assign(requests, { breakdown: breakdownRequests });
}

async function tick(ms: number) {
  await act(() => vi.advanceTimersByTimeAsync(ms));
}

beforeEach(() => {
  vi.useFakeTimers({ now: NOW, shouldAdvanceTime: true });
  vi.stubEnv("NEXT_PUBLIC_INGEST_URL", "http://localhost:8081");
  toasts.success.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllEnvs();
});

describe("IntegrationOnboarding", () => {
  it("switches to success after first event", async () => {
    const requests = mockApi({
      health: (n) => health(n < 2 ? {} : { first_event_at: "2026-10-13T11:32:05Z" }),
    });
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    const status = await screen.findByRole("status", { name: "Первое событие" });
    expect(status).toHaveTextContent("Ждём первое событие… Проверяем каждые 3 секунды.");
    expect(requests).toHaveLength(1);
    expect(requests[0]?.get("hours")).toBe("24");

    await tick(3000);
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(status).toHaveTextContent("Ждём первое событие…");

    await tick(3000);
    await waitFor(() => expect(requests).toHaveLength(3));
    await waitFor(() =>
      expect(screen.getByRole("status", { name: "Первое событие" })).toHaveTextContent(
        "Первое событие получено 14:32:05 — Notes iOS подключено!",
      ),
    );
    expect(toasts.success).toHaveBeenCalledWith("Первое событие получено");
    expect(screen.getByRole("link", { name: "Открыть Аудиторию" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}`,
    );

    // После первого события опрос — раз в 30 с.
    await tick(3000);
    expect(requests).toHaveLength(3);
    await tick(27_000);
    await waitFor(() => expect(requests).toHaveLength(4));
  });

  it("shows the oldest active key, endpoint and snippets", async () => {
    mockApi({
      keys: [
        writeKey({
          id: "k3",
          key: "pk_CCCCccccCCCCccccCCCCccccCCCCcccc",
          created_at: "2026-10-12T00:00:00Z",
        }),
        writeKey({
          id: "k1",
          key: "pk_AAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaa",
          created_at: "2026-09-01T00:00:00Z",
          revoked_at: "2026-09-02T00:00:00Z",
        }),
        writeKey({
          id: "k2",
          key: "pk_BBBBbbbbBBBBbbbbBBBBbbbbBBBBbbbb",
          created_at: "2026-10-01T00:00:00Z",
        }),
      ],
    });
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    expect(await screen.findByRole("textbox", { name: "write-ключ" })).toHaveValue(
      "pk_BBBBbbbbBBBBbbbbBBBBbbbbBBBBbbbb",
    );
    expect(screen.getByRole("textbox", { name: "эндпоинт" })).toHaveValue(
      "http://localhost:8081/v1/events",
    );
    expect(screen.getByRole("region", { name: "iOS" })).toBeInTheDocument();

    const swift = screen.getByLabelText("Пример кода на Swift");
    expect(swift.textContent).toContain('"pk_BBBBbbbbBBBBbbbbBBBBbbbbBBBBbbbb"');
    expect(swift.textContent).toContain('URL(string: "http://localhost:8081/v1/events")!');
  });

  it("hides snippets when every write key is revoked", async () => {
    mockApi({ keys: [writeKey({ revoked_at: "2026-10-13T11:10:00Z" })] });
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    const section = await screen.findByRole("region", { name: "iOS" });
    const alert = await within(section).findByRole("alert");
    expect(alert).toHaveTextContent("Для iOS нет активного ключа");
    expect(within(alert).getByRole("link", { name: "Создать ключ для iOS" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/settings#write-keys`,
    );
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "write-ключ" })).not.toBeInTheDocument();
    // Ожидание первого события продолжается и без ключа.
    expect(screen.getByRole("status", { name: "Первое событие" })).toHaveTextContent(
      "Ждём первое событие…",
    );
  });

  it("shows a section per platform", async () => {
    mockApi({
      platforms: ["ios", "web"],
      keys: [
        writeKey({
          id: "k2",
          key: "pk_K2K2k2k2K2K2k2k2K2K2k2k2K2K2k2k2",
          platform: null,
          created_at: "2026-09-01T00:00:00Z",
        }),
        writeKey({
          id: "k1",
          key: "pk_K1K1k1k1K1K1k1k1K1K1k1k1K1K1k1k1",
          platform: "ios",
          created_at: "2026-10-01T00:00:00Z",
        }),
      ],
    });
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    const ios = await screen.findByRole("region", { name: "iOS" });
    const web = screen.getByRole("region", { name: "Веб" });
    expect(within(ios).getByRole("textbox", { name: "write-ключ" })).toHaveValue(
      "pk_K1K1k1k1K1K1k1k1K1K1k1k1K1K1k1k1",
    );
    expect(within(ios).getByRole("tab", { name: "Swift" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(within(web).getByRole("textbox", { name: "write-ключ" })).toHaveValue(
      "pk_K2K2k2k2K2K2k2k2K2K2k2k2K2K2k2k2",
    );
    expect(within(web).getByRole("tab", { name: "JavaScript" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(within(web).getByLabelText("Пример кода на JavaScript").textContent).toContain(
      'platform: "web",',
    );
    // Эндпоинт общий для всех платформ.
    expect(screen.getAllByRole("textbox", { name: "эндпоинт" })).toHaveLength(1);
    expect(screen.getByRole("link", { name: "Добавить платформу" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/settings#platforms`,
    );
  });

  it("warns when a platform has no key", async () => {
    mockApi({
      platforms: ["ios", "android"],
      keys: [writeKey({ platform: "ios" })],
    });
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    const android = await screen.findByRole("region", { name: "Android" });
    const alert = within(android).getByRole("alert");
    expect(alert).toHaveTextContent("Для Android нет активного ключа");
    expect(within(alert).getByRole("link", { name: "Создать ключ для Android" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/settings#write-keys`,
    );
    expect(within(android).queryByRole("tablist")).not.toBeInTheDocument();
    const ios = screen.getByRole("region", { name: "iOS" });
    expect(within(ios).queryByRole("alert")).not.toBeInTheDocument();
    expect(within(ios).getByRole("textbox", { name: "write-ключ" })).toHaveValue(KEY);
  });

  it("marks platforms observed in the last 48 hours as received", async () => {
    const requests = mockApi({
      platforms: ["ios", "android", "web"],
      keys: [writeKey()],
      observed: { production: ["ios", "linux"], development: ["web"] },
    });
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    await waitFor(() =>
      expect(screen.getByRole("status", { name: "Статус iOS" })).toHaveTextContent(
        "Получено первое событие",
      ),
    );
    await waitFor(() =>
      expect(screen.getByRole("status", { name: "Статус Веб" })).toHaveTextContent(
        "Получено первое событие",
      ),
    );
    expect(screen.getByRole("status", { name: "Статус Android" })).toHaveTextContent(
      "Ждём первое событие с Android",
    );
    // Неявная платформа (linux) секцию не получает.
    expect(screen.queryByRole("region", { name: "Linux" })).not.toBeInTheDocument();

    // Вчера и сегодня по часовому поясу приложения (Europe/Moscow), оба окружения.
    const envs = requests.breakdown.map((p) => p.get("environment")).sort();
    expect(envs).toEqual(["development", "production"]);
    for (const params of requests.breakdown) {
      expect(params.get("dimension")).toBe("platform");
      expect(params.get("from")).toBe("2026-10-12");
      expect(params.get("to")).toBe("2026-10-13");
    }
  });

  it("polls platforms every 3 s while one is waiting and every 30 s once all are received", async () => {
    const waiting = mockApi({
      platforms: ["ios", "android"],
      keys: [writeKey()],
      observed: { production: ["ios"] },
    });
    const { unmount } = renderWithClient(<IntegrationPanel appId={APP_ID} />);
    await screen.findByRole("status", { name: "Статус Android" });
    await waitFor(() => expect(waiting.breakdown).toHaveLength(2));
    await tick(3000);
    await waitFor(() => expect(waiting.breakdown).toHaveLength(4));
    unmount();

    const received = mockApi({
      platforms: ["ios", "android"],
      keys: [writeKey()],
      observed: { production: ["ios", "android"] },
    });
    renderWithClient(<IntegrationPanel appId={APP_ID} />);
    await waitFor(() =>
      expect(screen.getByRole("status", { name: "Статус Android" })).toHaveTextContent(
        "Получено первое событие",
      ),
    );
    await waitFor(() => expect(received.breakdown).toHaveLength(2));
    await tick(3000);
    expect(received.breakdown).toHaveLength(2);
    await tick(27_000);
    await waitFor(() => expect(received.breakdown).toHaveLength(4));
  });
});

describe("IntegrationOnboarding in English", () => {
  it("guides through the first event in English", async () => {
    mockApi({
      platforms: ["ios", "android"],
      keys: [writeKey({ platform: "ios" })],
      health: (n) => health(n < 1 ? {} : { first_event_at: "2026-10-13T11:32:05Z" }),
    });
    renderInEnglish(<IntegrationPanel appId={APP_ID} />);

    const status = await screen.findByRole("status", { name: "First event" });
    expect(status).toHaveTextContent("Waiting for the first event… Checking every 3 seconds.");
    expect(await screen.findByRole("heading", { name: "Endpoint" })).toBeInTheDocument();
    expect(screen.getByText("One for all platforms of the app")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "endpoint" })).toHaveValue(
      "http://localhost:8081/v1/events",
    );
    expect(screen.getByRole("status", { name: "iOS status" })).toHaveTextContent(
      "Waiting for the first event from iOS",
    );
    const android = screen.getByRole("region", { name: "Android" });
    expect(within(android).getByRole("alert")).toHaveTextContent("No active key for Android.");
    expect(screen.getByRole("link", { name: "Add platform" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/settings#platforms`,
    );

    await tick(3000);
    await waitFor(() =>
      expect(screen.getByRole("status", { name: "First event" })).toHaveTextContent(
        "First event received at 14:32:05 — Notes iOS is connected!",
      ),
    );
    expect(toasts.success).toHaveBeenCalledWith("First event received");
    expect(screen.getByRole("link", { name: "Open Audience" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}`,
    );
  });
});
