import { QueryClientProvider } from "@tanstack/react-query";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useListWriteKeys } from "@/lib/api/gen/endpoints/admin-keys/admin-keys";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import type { AppMerge } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { appMerge, errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { renderWithLocale } from "@/test/rtl";
import { MergeProgressBanner } from "./merge-progress-banner";

const APP_ID = "0192aaaa-0000-7000-8000-000000000027";
const MERGE_ID = "0192bbbb-0000-7000-8000-000000000027";

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

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

function merge(overrides: Partial<AppMerge>): AppMerge {
  return appMerge({ id: MERGE_ID, target_app_id: APP_ID, ...overrides });
}

function mockMerges(list: (n: number) => AppMerge[], retry?: () => Response) {
  const calls = { list: 0, retry: [] as string[], app: 0, keys: 0 };
  server.use(
    http.get(`*/admin/v1/apps/${APP_ID}/merges`, () => {
      const n = calls.list;
      calls.list += 1;
      return HttpResponse.json({ items: list(n) });
    }),
    http.post(`*/admin/v1/apps/${APP_ID}/merges/:mergeId/retry`, ({ params }) => {
      calls.retry.push(String(params.mergeId));
      if (retry) return retry();
      return HttpResponse.json(merge({ status: "pending", step: "copy" }));
    }),
    http.get(`*/v1/apps/${APP_ID}`, () => {
      calls.app += 1;
      return HttpResponse.json(makeApp({ id: APP_ID }));
    }),
    http.get(`*/admin/v1/apps/${APP_ID}/write-keys`, () => {
      calls.keys += 1;
      return HttpResponse.json({ items: [] });
    }),
  );
  return calls;
}

// Соседние потребители кэша приложения и его ключей: по ним видно, что баннер инвалидирует запросы
// после переноса ключей и после слияния. Инвалидация запроса без данных, который ещё в полёте, сливается
// с ним, поэтому тесты монтируют баннер, когда у обоих запросов уже есть данные.
function AppProbe() {
  const app = useGetApp(APP_ID);
  const keys = useListWriteKeys(APP_ID);
  return app.data && keys.data ? <span>probe: loaded</span> : null;
}

async function renderWithLoadedProbe() {
  const { rerender } = renderWithClient(<AppProbe />);
  await screen.findByText("probe: loaded");
  rerender(
    <>
      <AppProbe />
      <MergeProgressBanner appId={APP_ID} />
    </>,
  );
}

async function tick(ms: number) {
  await act(() => vi.advanceTimersByTimeAsync(ms));
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  toasts.success.mockReset();
  toasts.error.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("MergeProgressBanner", () => {
  it("shows steps and retries", async () => {
    const calls = mockMerges((n) => {
      if (n === 0) return [merge({ status: "running", step: "wait_keys" })];
      if (n === 1) {
        return [merge({ status: "failed", step: "copy", error: "ClickHouse недоступен" })];
      }
      return [merge({ status: "pending", step: "copy", copied_events: 500, total_events: 1000 })];
    });
    renderWithClient(<MergeProgressBanner appId={APP_ID} />);

    const banner = await screen.findByRole("status");
    expect(banner).toHaveAttribute("aria-live", "polite");
    expect(banner).toHaveTextContent(
      "Объединение с «Notes Android»: ждём, пока приём событий перечитает ключи (до 17 мин)",
    );

    await tick(5000);
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        "Объединение прервалось: ClickHouse недоступен",
      ),
    );
    expect(screen.getByRole("status")).toHaveAttribute("aria-live", "polite");

    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(screen.getByRole("button", { name: "Повторить" }));

    await waitFor(() => expect(calls.retry).toEqual([MERGE_ID]));
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        /Объединение с «Notes Android»: перенесено 500 из 1\s000 событий/,
      ),
    );
    expect(screen.queryByRole("button", { name: "Повторить" })).toBeNull();
  });

  it("announces the finished merge and refreshes the app data", async () => {
    const calls = mockMerges((n) =>
      n === 0
        ? [merge({ status: "running", step: "reclose", copied_events: 1000, total_events: 1000 })]
        : [merge({ status: "done", step: "done", copied_events: 1000, total_events: 1000 })],
    );
    await renderWithLoadedProbe();

    await screen.findByRole("status");
    // Слияние впервые увидено после move_keys: приложение и ключи перечитаны один раз.
    await waitFor(() => expect(calls.app).toBe(2));
    await waitFor(() => expect(calls.keys).toBe(2));

    await tick(5000);
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Приложения объединены"));
    expect(screen.queryByRole("status")).toBeNull();
    await waitFor(() => expect(calls.app).toBe(3));
    await waitFor(() => expect(calls.keys).toBe(3));
  });

  it("refreshes the app and its write keys once the keys are moved", async () => {
    const calls = mockMerges((n) =>
      n === 0
        ? [merge({ status: "running", step: "move_keys" })]
        : [merge({ status: "running", step: "wait_keys" })],
    );
    await renderWithLoadedProbe();

    await screen.findByRole("status");
    expect(calls.app).toBe(1);
    expect(calls.keys).toBe(1);
    await tick(1000);
    // Ключи ещё не перенесены: перечитывать нечего.
    expect(calls.app).toBe(1);
    expect(calls.keys).toBe(1);

    await tick(5000);
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("ждём, пока приём событий"),
    );
    await waitFor(() => expect(calls.app).toBe(2));
    await waitFor(() => expect(calls.keys).toBe(2));

    // Следующие опросы того же шага ничего не перечитывают.
    await tick(5000);
    await waitFor(() => expect(calls.list).toBeGreaterThanOrEqual(3));
    expect(calls.app).toBe(2);
    expect(calls.keys).toBe(2);
  });

  it("stays hidden and stops polling without an active merge", async () => {
    const calls = mockMerges(() => [merge({ status: "done", step: "done" })]);
    renderWithClient(<MergeProgressBanner appId={APP_ID} />);

    await waitFor(() => expect(calls.list).toBe(1));
    await tick(15_000);

    expect(calls.list).toBe(1);
    expect(screen.queryByRole("status")).toBeNull();
    expect(toasts.success).not.toHaveBeenCalled();
  });

  it("toasts when the retry is rejected", async () => {
    mockMerges(
      () => [merge({ status: "failed", step: "copy", error: null })],
      () => HttpResponse.json(errorBody("merge_in_progress"), { status: 409 }),
    );
    renderWithClient(<MergeProgressBanner appId={APP_ID} />);

    expect(await screen.findByRole("status")).toHaveTextContent(
      "Объединение прервалось: причина неизвестна",
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(screen.getByRole("button", { name: "Повторить" }));

    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith(
        "Приложение участвует в объединении — дождитесь завершения",
      ),
    );
  });
});

describe("MergeProgressBanner in English", () => {
  it("shows merge steps in English", async () => {
    const calls = mockMerges((n) => {
      if (n === 0) return [merge({ status: "running", step: "wait_keys" })];
      if (n === 1) return [merge({ status: "failed", step: "copy", error: null })];
      return [merge({ status: "pending", step: "copy", copied_events: 500, total_events: 1000 })];
    });
    renderInEnglish(<MergeProgressBanner appId={APP_ID} />);

    expect(await screen.findByRole("status")).toHaveTextContent(
      "Merging with “Notes Android”: waiting for event intake to reload the keys (up to 17 min)",
    );

    await tick(5000);
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("Merge failed: unknown reason"),
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(screen.getByRole("button", { name: "Try again" }));

    await waitFor(() => expect(calls.retry).toEqual([MERGE_ID]));
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        /Merging with “Notes Android”: moved 500 of 1,000 events/,
      ),
    );
  });

  it("uses the singular for a single event", async () => {
    mockMerges(() => [
      merge({ status: "running", step: "copy", copied_events: 0, total_events: 1 }),
    ]);
    renderInEnglish(<MergeProgressBanner appId={APP_ID} />);

    expect(await screen.findByRole("status")).toHaveTextContent(
      "Merging with “Notes Android”: moved 0 of 1 event",
    );
  });
});

describe("MergeProgressBanner plurals in Russian", () => {
  it("agrees the event count with the total", async () => {
    mockMerges(() => [
      merge({ status: "running", step: "copy", copied_events: 0, total_events: 21 }),
    ]);
    renderWithClient(<MergeProgressBanner appId={APP_ID} />);

    expect(await screen.findByRole("status")).toHaveTextContent(
      "Объединение с «Notes Android»: перенесено 0 из 21 события",
    );
  });
});
