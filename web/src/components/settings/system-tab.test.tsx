import { screen, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import type { SystemStatus } from "@/lib/api/gen/model";
import { errorBody, systemStatus } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { SystemTab } from "./system-tab";

const NOW = new Date("2026-10-13T12:00:00Z");

function mockStatus(status: SystemStatus) {
  server.use(http.get("*/admin/v1/system/status", () => HttpResponse.json(status)));
}

describe("SystemTab", () => {
  it("shows healthy components, backup, alerts and jobs", async () => {
    mockStatus(systemStatus());
    renderWithClient(<SystemTab timezone="Europe/Moscow" now={NOW} />);

    const ch = await screen.findByRole("group", { name: "ClickHouse" });
    expect(ch).toHaveTextContent("OK");
    expect(ch).toHaveTextContent("свободно 71 %");
    expect(ch).toHaveTextContent("1 204 551 902");
    expect(screen.getByRole("group", { name: "Postgres" })).toHaveTextContent("OK");
    const backup = screen.getByRole("group", { name: "Бэкап" });
    expect(backup).toHaveTextContent("increment-2026-10-13");
    expect(backup).not.toHaveTextContent("старше 26 часов");

    const alerts = screen.getByRole("table", { name: "Алерты" });
    expect(within(alerts).getByText("disk_free")).toBeInTheDocument();
    expect(within(alerts).getByText("Норма")).toBeInTheDocument();
    const jobs = screen.getByRole("table", { name: "Задачи" });
    expect(within(jobs).getByText("system_check")).toBeInTheDocument();
    expect(within(jobs).getByText("ok")).toBeInTheDocument();
  });

  it("highlights failures, low disk, stale backup and firing alerts", async () => {
    mockStatus(
      systemStatus({
        clickhouse: { ok: false, disk_free_pct: 12, events_rows: null },
        postgres: { ok: false },
        backup: { last_backup_at: "2026-10-12T06:00:00Z", last_backup_name: "full-2026-10-12" },
        alerts: [
          {
            key: "insert_errors",
            firing: true,
            first_fired_at: "2026-10-13T11:05:00Z",
            last_notified_at: "2026-10-13T11:05:00Z",
            last_value: "3",
          },
        ],
        jobs: [],
      }),
    );
    renderWithClient(<SystemTab timezone="Europe/Moscow" now={NOW} />);

    const ch = await screen.findByRole("group", { name: "ClickHouse" });
    expect(ch).toHaveTextContent("Недоступен");
    expect(ch).toHaveTextContent("свободно 12 %");
    expect(screen.getByRole("group", { name: "Postgres" })).toHaveTextContent("Недоступен");
    expect(screen.getByRole("group", { name: "Бэкап" })).toHaveTextContent("Бэкап старше 26 часов");
    expect(screen.getByText("Сработал с 14:05")).toBeInTheDocument();
    expect(screen.getByText("Запусков ещё не было")).toBeInTheDocument();
  });

  it("warns when there is no backup at all", async () => {
    mockStatus(systemStatus({ backup: { last_backup_at: null, last_backup_name: null } }));
    renderWithClient(<SystemTab timezone="Europe/Moscow" now={NOW} />);

    expect(await screen.findByRole("group", { name: "Бэкап" })).toHaveTextContent(
      "Бэкап старше 26 часов",
    );
  });

  it("shows an error state", async () => {
    server.use(
      http.get("*/admin/v1/system/status", () =>
        HttpResponse.json(errorBody("internal_error"), { status: 500 }),
      ),
    );
    renderWithClient(<SystemTab timezone="Europe/Moscow" now={NOW} />);
    expect(await screen.findByRole("button", { name: "Повторить" })).toBeInTheDocument();
  });
});

describe("SystemTab in English", () => {
  it("describes healthy components, alerts and jobs in English", async () => {
    mockStatus(systemStatus());
    renderWithClient(<SystemTab timezone="Europe/Moscow" now={NOW} />, { locale: "en" });

    expect(await screen.findByRole("heading", { name: "System" })).toBeInTheDocument();
    const ch = await screen.findByRole("group", { name: "ClickHouse" });
    expect(ch).toHaveTextContent("OK");
    expect(ch).toHaveTextContent("71% free");
    expect(ch).toHaveTextContent("1,204,551,902 events");
    const backup = screen.getByRole("group", { name: "Backup" });
    expect(backup).toHaveTextContent("Backup:");
    expect(backup).toHaveTextContent("Oct 13, 2026, 03:12");

    const alerts = screen.getByRole("table", { name: "Alerts" });
    expect(
      within(alerts)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Alert", "State", "Value"]);
    expect(within(alerts).getByText("Normal")).toBeInTheDocument();
    const jobs = screen.getByRole("table", { name: "Jobs" });
    expect(
      within(jobs)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Job", "Last run", "Status"]);
  });

  it("describes failures and empty lists in English", async () => {
    mockStatus(
      systemStatus({
        clickhouse: { ok: false, disk_free_pct: 12, events_rows: null },
        postgres: { ok: false },
        backup: { last_backup_at: null, last_backup_name: null },
        alerts: [],
        jobs: [],
      }),
    );
    renderWithClient(<SystemTab timezone="Europe/Moscow" now={NOW} />, { locale: "en" });

    const ch = await screen.findByRole("group", { name: "ClickHouse" });
    expect(ch).toHaveTextContent("Unavailable");
    expect(ch).toHaveTextContent("12% free");
    const backup = screen.getByRole("group", { name: "Backup" });
    expect(backup).toHaveTextContent("no data");
    expect(backup).toHaveTextContent("Backup is older than 26 hours");
    expect(screen.getByText("Alerts haven't been checked yet")).toBeInTheDocument();
    expect(screen.getByText("No runs yet")).toBeInTheDocument();
  });

  it("shows when an alert started firing in English", async () => {
    mockStatus(
      systemStatus({
        alerts: [
          {
            key: "insert_errors",
            firing: true,
            first_fired_at: "2026-10-13T11:05:00Z",
            last_notified_at: "2026-10-13T11:05:00Z",
            last_value: "3",
          },
        ],
      }),
    );
    renderWithClient(<SystemTab timezone="Europe/Moscow" now={NOW} />, { locale: "en" });

    expect(await screen.findByText("Firing since 14:05")).toBeInTheDocument();
  });
});
