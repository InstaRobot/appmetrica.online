import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Breakdown } from "@/lib/api/gen/model";
import { BreakdownTable } from "./breakdown-table";

const versions: Breakdown = {
  dimension: "app_version",
  metric: "active",
  total: 41230,
  rows: [
    { value: "2.1.1", count: 24001, share: 0.5821 },
    { value: "2.1.0", count: 12330, share: 0.299 },
    { value: "", count: 15, share: 0.0004 },
  ],
};

const noop = () => {};

describe("BreakdownTable", () => {
  it("lists values with counts and shares", () => {
    render(
      <BreakdownTable
        dimension="app_version"
        onDimensionChange={noop}
        metric="active"
        onMetricChange={noop}
        data={versions}
      />,
    );
    const table = screen.getByRole("table", { name: "Разбивка: Версия приложения" });
    const rows = within(table).getAllByRole("row");
    expect(rows.map((r) => r.textContent?.replace(/ /g, " "))).toEqual([
      "ЗначениеАктивныеДоля",
      "2.1.124 00158,2 %",
      "2.1.012 33029,9 %",
      "Не определено150 %",
    ]);
    expect(screen.getByText(/Сумма долей может превышать 100/)).toBeInTheDocument();
  });

  it("switches dimension and metric", async () => {
    const onDimensionChange = vi.fn();
    const onMetricChange = vi.fn();
    const user = userEvent.setup();
    render(
      <BreakdownTable
        dimension="app_version"
        onDimensionChange={onDimensionChange}
        metric="active"
        onMetricChange={onMetricChange}
        data={versions}
      />,
    );
    const dims = screen.getByRole("radiogroup", { name: "Измерение" });
    expect(
      within(dims)
        .getAllByRole("radio")
        .map((r) => r.textContent),
    ).toEqual([
      "Версия приложения",
      "ОС",
      "Версия ОС",
      "Устройство",
      "Тип устройства",
      "Страна",
      "Город",
      "Язык",
      "Платформа",
    ]);
    await user.click(within(dims).getByRole("radio", { name: "Страна" }));
    expect(onDimensionChange).toHaveBeenCalledWith("country");

    await user.click(screen.getByRole("combobox", { name: "Метрика" }));
    await user.click(await screen.findByRole("option", { name: "Сессии" }));
    expect(onMetricChange).toHaveBeenCalledWith("sessions");
  });

  it("adds a filter by a row of a filterable dimension", async () => {
    const onRowClick = vi.fn();
    const user = userEvent.setup();
    const { rerender } = render(
      <BreakdownTable
        dimension="app_version"
        onDimensionChange={noop}
        metric="new"
        onMetricChange={noop}
        data={{ ...versions, metric: "new" }}
        onRowClick={onRowClick}
      />,
    );
    expect(screen.queryByText(/Сумма долей/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Фильтр: Версия приложения 2.1.0" }));
    expect(onRowClick).toHaveBeenCalledWith("2.1.0");
    // Пустое значение отфильтровать нельзя.
    expect(screen.queryByRole("button", { name: /Не определено/ })).not.toBeInTheDocument();

    rerender(
      <BreakdownTable
        dimension="city"
        onDimensionChange={noop}
        metric="active"
        onMetricChange={noop}
        data={{
          dimension: "city",
          metric: "active",
          total: 1,
          rows: [{ value: "Moscow", count: 1, share: 1 }],
        }}
        onRowClick={onRowClick}
      />,
    );
    expect(screen.getByText("Moscow")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Фильтр: Город/ })).not.toBeInTheDocument();
  });

  it("shows loading and empty states", () => {
    const { rerender } = render(
      <BreakdownTable
        dimension="country"
        onDimensionChange={noop}
        metric="active"
        onMetricChange={noop}
        data={undefined}
        loading
      />,
    );
    expect(screen.getByRole("status", { name: "Загрузка…" })).toBeInTheDocument();

    rerender(
      <BreakdownTable
        dimension="country"
        onDimensionChange={noop}
        metric="active"
        onMetricChange={noop}
        data={{ dimension: "country", metric: "active", total: 0, rows: [] }}
      />,
    );
    expect(screen.getByText("Нет данных за выбранный период")).toBeInTheDocument();
  });
});
