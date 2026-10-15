import { QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import type { DeletionRequest } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { appSummary, deletionRequest, errorBody } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { renderWithLocale } from "@/test/rtl";
import { DeletionsTab } from "./deletions-tab";

const NOTES = appSummary({ id: "0192aaaa-0000-7000-8000-000000000047", name: "Notes iOS" });
const INSTALL = "5d8e2c1a-0b3f-4e5f-9a7b-0c1d2e3f4e5f";

function mockRequests(items: DeletionRequest[]) {
  server.use(
    http.get("*/v1/apps", () => HttpResponse.json({ items: [NOTES] })),
    http.get("*/admin/v1/deletion-requests", () => HttpResponse.json({ items })),
  );
}

describe("DeletionsTab", () => {
  it("lists requests with readable types, sources and statuses", async () => {
    mockRequests([
      deletionRequest({
        app_id: NOTES.id,
        subject_type: "install",
        subject_value: INSTALL,
        source: "api",
        status: "done",
        completed_at: "2026-10-12T13:00:00Z",
      }),
      deletionRequest({
        app_id: "0192aaaa-0000-7000-8000-0000000000ff",
        subject_type: "user",
        subject_value: "42",
        source: "dashboard",
        status: "failed",
        error: "clickhouse: timeout",
      }),
    ]);
    renderWithClient(<DeletionsTab timezone="Europe/Moscow" />);

    const table = await screen.findByRole("table", { name: "Запросы на удаление" });
    const rows = within(table).getAllByRole("row").slice(1);
    await waitFor(() => expect(rows[0]).toHaveTextContent("Notes iOS"));
    expect(rows[0]).toHaveTextContent("Устройство");
    expect(rows[0]).toHaveTextContent("5d8e…4e5f");
    expect(rows[0]).toHaveTextContent("API");
    expect(rows[0]).toHaveTextContent("Удалено");
    expect(rows[1]).toHaveTextContent("Пользователь");
    expect(rows[1]).toHaveTextContent("Дашборд");
    expect(rows[1]).toHaveTextContent("Ошибка");
    expect(rows[1]).toHaveTextContent("clickhouse: timeout");
    // Приложение уже удалено — показываем сокращённый id.
    expect(rows[1]).toHaveTextContent("0192…00ff");
  });

  it("says when there were no requests", async () => {
    mockRequests([]);
    renderWithClient(<DeletionsTab timezone="Europe/Moscow" />);
    expect(await screen.findByText("Запросов на удаление не было")).toBeInTheDocument();
  });

  it("shows an error state", async () => {
    server.use(
      http.get("*/v1/apps", () => HttpResponse.json({ items: [] })),
      http.get("*/admin/v1/deletion-requests", () =>
        HttpResponse.json(errorBody("internal_error"), { status: 500 }),
      ),
    );
    renderWithClient(<DeletionsTab timezone="Europe/Moscow" />);
    expect(await screen.findByRole("button", { name: "Повторить" })).toBeInTheDocument();
  });
});

function renderEnglish(ui: ReactElement) {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return renderWithLocale(<QueryClientProvider client={client}>{ui}</QueryClientProvider>, "en");
}

describe("DeletionsTab in English", () => {
  it("lists requests with English headers, types, sources and statuses", async () => {
    mockRequests([
      deletionRequest({
        app_id: NOTES.id,
        subject_type: "install",
        subject_value: INSTALL,
        source: "api",
        status: "done",
        completed_at: "2026-10-12T13:00:00Z",
      }),
      deletionRequest({
        app_id: NOTES.id,
        subject_type: "user",
        source: "dashboard",
        status: "pending",
      }),
    ]);
    renderEnglish(<DeletionsTab timezone="Europe/Moscow" />);

    expect(await screen.findByRole("heading", { name: "Deletions" })).toBeInTheDocument();
    const table = await screen.findByRole("table", { name: "Deletion requests" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["App", "Type", "Value", "Source", "Status", "Created", "Completed", "Error"]);
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("Device");
    expect(rows[0]).toHaveTextContent("Deleted");
    expect(rows[0]).toHaveTextContent("Oct 12, 2026, 16:00");
    expect(rows[1]).toHaveTextContent("User");
    expect(rows[1]).toHaveTextContent("Dashboard");
    expect(rows[1]).toHaveTextContent("Queued");
  });

  it("says in English when there were no requests", async () => {
    mockRequests([]);
    renderEnglish(<DeletionsTab timezone="Europe/Moscow" />);
    expect(await screen.findByText("There have been no deletion requests")).toBeInTheDocument();
  });
});
