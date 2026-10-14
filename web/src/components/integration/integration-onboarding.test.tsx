import { act, screen, waitFor, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IntegrationHealth, WriteKey } from "@/lib/api/gen/model";
import { app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { IntegrationPanel } from "./integration-panel";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

const APP_ID = "0192aaaa-0000-7000-8000-000000000010";
// 2026-10-13 14:32:05 по Москве.
const NOW = new Date("2026-10-13T11:32:05Z");
const KEY = "pk_7Hq2x9AbLm3Np4Qr5St6Uv7Wx8Yz9Ab0";

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
    created_at: "2026-10-13T11:00:00Z",
    revoked_at: null,
    last_used_at: null,
    ...overrides,
  };
}

type MockOptions = {
  keys?: WriteKey[];
  /** Ответ на n-й (с нуля) запрос здоровья. */
  health?: (n: number) => IntegrationHealth;
};

function mockApi(opts: MockOptions = {}) {
  const requests: URLSearchParams[] = [];
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(makeApp({ id: APP_ID, name: "Notes iOS", platform: "ios" })),
    ),
    http.get(`*/admin/v1/apps/${APP_ID}/write-keys`, () =>
      HttpResponse.json({ items: opts.keys ?? [writeKey()] }),
    ),
    http.get(`*/v1/apps/${APP_ID}/integration/health`, ({ request }) => {
      const n = requests.length;
      requests.push(new URL(request.url).searchParams);
      return HttpResponse.json(opts.health ? opts.health(n) : health());
    }),
  );
  return requests;
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
    expect(screen.getByRole("heading", { name: "1. Ключ приложения" })).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "2. Отправьте первое событие" }),
    ).toBeInTheDocument();

    const swift = screen.getByLabelText("Пример кода на Swift");
    expect(swift.textContent).toContain('"pk_BBBBbbbbBBBBbbbbBBBBbbbbBBBBbbbb"');
    expect(swift.textContent).toContain('URL(string: "http://localhost:8081/v1/events")!');
  });

  it("hides snippets when every write key is revoked", async () => {
    mockApi({ keys: [writeKey({ revoked_at: "2026-10-13T11:10:00Z" })] });
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    const alert = await screen.findByText(
      "Нет активного write-ключа. Создайте ключ в настройках приложения.",
    );
    const link = within(alert.closest("[role=alert]") as HTMLElement).getByRole("link", {
      name: "Настройки приложения",
    });
    expect(link).toHaveAttribute("href", `/apps/${APP_ID}/settings`);
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "write-ключ" })).not.toBeInTheDocument();
    // Ожидание первого события продолжается и без ключа.
    expect(screen.getByRole("status", { name: "Первое событие" })).toHaveTextContent(
      "Ждём первое событие…",
    );
  });
});
