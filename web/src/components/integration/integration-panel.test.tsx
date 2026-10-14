import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IntegrationHealth } from "@/lib/api/gen/model";
import { formatDateTime } from "@/lib/format";
import { errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { IntegrationPanel } from "./integration-panel";

const APP_ID = "0192aaaa-0000-7000-8000-000000000011";
const KEY = "pk_7Hq2x9AbLm3Np4Qr5St6Uv7Wx8Yz9Ab0";
const NBSP = "\u00a0";

function health(overrides: Partial<IntegrationHealth> = {}): IntegrationHealth {
  return {
    hours: 24,
    first_event_at: "2026-09-01T10:00:00Z",
    accepted_total: 412905,
    rejected_total: 1204,
    hourly: Array.from({ length: 24 }, (_, i) => ({
      hour: new Date(Date.UTC(2026, 9, 12, 12 + i)).toISOString(),
      accepted: 17000,
      rejected: 50,
    })),
    reasons: [
      {
        code: "event_too_old",
        count: 903,
        last_seen: "2026-10-13T11:20:00Z",
        example_payload: '{"name":"session_start"}',
        example_message: "timestamp is older than 7 days",
      },
    ],
    versions: [
      { app_version: "2.1.1", accepted: 300112, rejected: 12, last_seen: "2026-10-13T11:32:00Z" },
    ],
    ...overrides,
  };
}

function mockApi(respond: (n: number) => IntegrationHealth | { status: number }) {
  let n = 0;
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(makeApp({ id: APP_ID, platform: "android", platforms: ["android"] })),
    ),
    http.get(`*/admin/v1/apps/${APP_ID}/write-keys`, () =>
      HttpResponse.json({
        items: [
          {
            id: "k1",
            app_id: APP_ID,
            key: KEY,
            label: "",
            platform: null,
            created_at: "2026-09-01T00:00:00Z",
            revoked_at: null,
            last_used_at: null,
          },
        ],
      }),
    ),
    http.get(`*/v1/apps/${APP_ID}/integration/health`, () => {
      const body = respond(n++);
      if ("status" in body) {
        return HttpResponse.json(errorBody("internal_error"), { status: body.status });
      }
      return HttpResponse.json(body);
    }),
  );
}

beforeEach(() => {
  vi.stubEnv("NEXT_PUBLIC_INGEST_URL", "http://localhost:8081");
});

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("IntegrationPanel", () => {
  it("shows ingest health after the first event", async () => {
    mockApi(() => health());
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    // toHaveTextContent сводит U+00A0 к пробелу, поэтому числа сверяются по textContent.
    const accepted = await screen.findByRole("group", { name: "Принято за 24 ч" });
    expect(accepted.textContent).toContain(`412${NBSP}905`);
    expect(screen.getByRole("group", { name: "Отклонено за 24 ч" }).textContent).toContain(
      `1${NBSP}204`,
    );
    const rate = screen.getByRole("group", { name: "Доля отказов" });
    expect(rate.textContent).toContain(`0,29${NBSP}%`);
    expect(within(rate).queryByRole("img")).not.toBeInTheDocument();

    expect(await screen.findByRole("img", { name: /^События по часам/ })).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Причины отказов" })).toHaveTextContent(
      "Событие старше 7 дней",
    );
    expect(screen.getByRole("table", { name: "По версиям приложения" })).toHaveTextContent("2.1.1");

    // Онбординг-блоки свёрнуты в раскрывающуюся секцию.
    expect(screen.queryByRole("status", { name: "Первое событие" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "write-ключ" })).not.toBeInTheDocument();
    const user = userEvent.setup();
    const toggle = screen.getByRole("button", { name: "Показать ключ и сниппеты" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    // Онбординг — ленивый чанк: ключ появляется после его загрузки.
    expect(await screen.findByRole("textbox", { name: "write-ключ" })).toHaveValue(KEY);
    expect(screen.getByRole("tab", { name: "Kotlin" })).toHaveAttribute("aria-selected", "true");
  });

  it("shows when the last event arrived, taken from the freshest app version", async () => {
    mockApi(() =>
      health({
        versions: [
          { app_version: "2.1.0", accepted: 10, rejected: 0, last_seen: "2026-10-13T09:05:00Z" },
          { app_version: "2.1.1", accepted: 20, rejected: 0, last_seen: "2026-10-13T11:32:00Z" },
        ],
      }),
    );
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    const last = await screen.findByRole("group", { name: "Последнее событие" });
    expect(last.textContent).toContain(
      formatDateTime("2026-10-13T11:32:00Z", makeApp({ id: APP_ID }).timezone),
    );
  });

  it("shows a dash for the last event when no version has events", async () => {
    mockApi(() => health({ versions: [] }));
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    expect(await screen.findByRole("group", { name: "Последнее событие" })).toHaveTextContent("—");
  });

  it("warns when the rejection rate reaches 1 %", async () => {
    mockApi(() => health({ accepted_total: 99, rejected_total: 1, reasons: [], versions: [] }));
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    const rate = await screen.findByRole("group", { name: "Доля отказов" });
    expect(rate.textContent).toContain(`1${NBSP}%`);
    expect(within(rate).getByRole("img", { name: "Доля отказов 1 % и выше" })).toBeInTheDocument();
    expect(screen.getByText("Отказов за 24 часа нет ✓")).toBeInTheDocument();
  });

  it("shows an error with retry", async () => {
    mockApi((n) => (n === 0 ? { status: 500 } : health()));
    renderWithClient(<IntegrationPanel appId={APP_ID} />);

    const retry = await screen.findByRole("button", { name: "Повторить" });
    expect(screen.getByText("Ошибка сервера. Повторите через минуту.")).toBeInTheDocument();
    await userEvent.setup().click(retry);
    await waitFor(() =>
      expect(screen.getByRole("group", { name: "Принято за 24 ч" })).toBeInTheDocument(),
    );
  });

  it("shows a skeleton while loading", () => {
    mockApi(() => health());
    renderWithClient(<IntegrationPanel appId={APP_ID} />);
    expect(screen.getAllByRole("status", { name: "Загрузка…" }).length).toBeGreaterThan(0);
  });
});
