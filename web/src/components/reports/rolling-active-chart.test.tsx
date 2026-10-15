import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { RollingActiveChart } from "./rolling-active-chart";

describe("RollingActiveChart", () => {
  it("describes DAU, WAU and MAU and shows them as a table", async () => {
    const user = userEvent.setup();
    render(
      <RollingActiveChart
        data={{
          points: [
            { day: "2026-10-11", dau: 12011, wau: 40100, mau: 98000 },
            { day: "2026-10-12", dau: 12480, wau: 41230, mau: 98411 },
          ],
        }}
      />,
    );
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^График DAU, WAU и MAU, 11–12\sокт\s2026, последний день: DAU 12\s480, WAU 41\s230, MAU 98\s411$/,
    );
    await user.click(screen.getByRole("button", { name: "Показать таблицей" }));
    const rows = within(screen.getByRole("table")).getAllByRole("row");
    expect(rows.map((r) => r.textContent?.replace(/\u00a0/g, " "))).toEqual([
      "ДеньDAUWAUMAU",
      "11 окт 202612 01140 10098 000",
      "12 окт 202612 48041 23098 411",
    ]);
  });

  it("says when there is nothing to show", () => {
    render(<RollingActiveChart data={{ points: [] }} />);
    expect(screen.getByText("Нет данных за выбранный период")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Показать таблицей" })).not.toBeInTheDocument();
  });
});

describe("RollingActiveChart in English", () => {
  it("describes the chart and the table in English", async () => {
    const user = userEvent.setup();
    renderWithLocale(
      <RollingActiveChart
        data={{
          points: [
            { day: "2026-10-11", dau: 12011, wau: 40100, mau: 98000 },
            { day: "2026-10-12", dau: 12480, wau: 41230, mau: 98411 },
          ],
        }}
      />,
      "en",
    );
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^DAU, WAU and MAU chart, Oct\s11–12, 2026, last day: DAU 12,480, WAU 41,230, MAU 98,411$/,
    );
    await user.click(screen.getByRole("button", { name: "Show as table" }));
    const rows = within(screen.getByRole("table")).getAllByRole("row");
    expect(rows.map((r) => r.textContent?.replace(/\u00a0/g, " "))).toEqual([
      "DayDAUWAUMAU",
      "Oct 11, 202612,01140,10098,000",
      "Oct 12, 202612,48041,23098,411",
    ]);
    expect(screen.getByRole("button", { name: "Hide table" })).toBeInTheDocument();
  });

  it("says when there is nothing to show in English", () => {
    renderWithLocale(<RollingActiveChart data={{ points: [] }} />, "en");
    expect(screen.getByText("No data for the selected period")).toBeInTheDocument();
  });
});
