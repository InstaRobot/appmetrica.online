import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { VersionsHealthTable } from "./versions-health-table";

const NBSP = "\u00a0";

describe("VersionsHealthTable", () => {
  it("shows rejection rate per version and warns at 1 %", () => {
    render(
      <VersionsHealthTable
        timezone="Europe/Moscow"
        versions={[
          {
            app_version: "2.1.1",
            accepted: 300112,
            rejected: 12,
            last_seen: "2026-10-13T11:32:00Z",
          },
          {
            app_version: "2.0.9",
            accepted: 98400,
            rejected: 1190,
            last_seen: "2026-10-13T11:31:00Z",
          },
          { app_version: "", accepted: 5, rejected: 0, last_seen: "2026-10-13T11:00:00Z" },
        ]}
      />,
    );

    const table = screen.getByRole("table", { name: "По версиям приложения" });
    const rows = within(table).getAllByRole("row");
    expect(
      within(rows[0] as HTMLElement)
        .getAllByRole("columnheader")
        .map((c) => c.textContent),
    ).toEqual(["Версия", "Принято", "Отклонено", "Доля отказов", "Последнее событие"]);

    const healthy = within(rows[1] as HTMLElement).getAllByRole("cell");
    expect(healthy.map((c) => c.textContent)).toEqual([
      "2.1.1",
      `300${NBSP}112`,
      "12",
      `0,004${NBSP}%`,
      "14:32",
    ]);
    expect(within(rows[1] as HTMLElement).queryByRole("img")).not.toBeInTheDocument();

    const bad = within(rows[2] as HTMLElement).getAllByRole("cell");
    expect(bad[3]?.textContent).toBe(`1,2${NBSP}%`);
    expect(
      within(bad[3] as HTMLElement).getByRole("img", { name: "Доля отказов 1 % и выше" }),
    ).toBeInTheDocument();

    expect(within(rows[3] as HTMLElement).getAllByRole("cell")[0]).toHaveTextContent("(не задана)");
  });

  it("says when there are no versions", () => {
    render(<VersionsHealthTable timezone="UTC" versions={[]} />);
    expect(screen.getByText("Событий за 24 часа нет")).toBeInTheDocument();
  });
});
