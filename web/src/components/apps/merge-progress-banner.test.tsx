import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import type { AppMerge } from "@/lib/api/gen/model";
import { appMerge, errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { MergeProgressBanner } from "./merge-progress-banner";

const APP_ID = "0192aaaa-0000-7000-8000-000000000027";
const MERGE_ID = "0192bbbb-0000-7000-8000-000000000027";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

function merge(overrides: Partial<AppMerge>): AppMerge {
  return appMerge({ id: MERGE_ID, target_app_id: APP_ID, ...overrides });
}

function mockMerges(list: (n: number) => AppMerge[], retry?: () => Response) {
  const calls = { list: 0, retry: [] as string[], app: 0 };
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
  );
  return calls;
}

// Соседний потребитель кэша приложения: по нему видно, что баннер инвалидирует запросы после слияния.
function AppProbe() {
  useGetApp(APP_ID);
  return null;
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
    renderWithClient(
      <>
        <AppProbe />
        <MergeProgressBanner appId={APP_ID} />
      </>,
    );

    await screen.findByRole("status");
    await waitFor(() => expect(calls.app).toBe(1));

    await tick(5000);
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Приложения объединены"));
    expect(screen.queryByRole("status")).toBeNull();
    await waitFor(() => expect(calls.app).toBe(2));
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
