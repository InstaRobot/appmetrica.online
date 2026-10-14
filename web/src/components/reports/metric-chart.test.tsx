import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { TimeSeries } from "@/lib/api/gen/model";
import { MetricChart } from "./metric-chart";

const day = (d: number) => `2026-10-${String(d).padStart(2, "0")}T00:00:00+03:00`;

const daily: TimeSeries = {
  metric: "active",
  granularity: "day",
  points: [6, 7, 8, 9, 10, 11, 12].map((d) => ({ ts: day(d), value: d === 7 ? 12480 : 9000 + d })),
};

describe("MetricChart", () => {
  it("describes the chart for screen readers", () => {
    render(
      <MetricChart series={daily} granularity="day" metric="active" timezone="Europe/Moscow" />,
    );
    expect(
      screen.getByRole("img", {
        name: /^График активных пользователей по дням, 6–12\sокт\s2026, максимум 12\s480 7\sокт$/,
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Нет событий за выбранный период")).not.toBeInTheDocument();
  });

  it("shows the points as a table on demand, with the previous period", async () => {
    const user = userEvent.setup();
    const series: TimeSeries = {
      metric: "sessions",
      granularity: "hour",
      points: [
        { ts: "2026-10-12T00:00:00+03:00", value: 10 },
        { ts: "2026-10-12T01:00:00+03:00", value: 1204 },
      ],
      compare_points: [
        { ts: "2026-10-11T00:00:00+03:00", value: 8 },
        { ts: "2026-10-11T01:00:00+03:00", value: 900 },
      ],
    };
    render(
      <MetricChart series={series} granularity="hour" metric="sessions" timezone="Europe/Moscow" />,
    );
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^График сессий по часам, 12\sокт\s2026, максимум 1\s204 12\sокт, 01:00$/,
    );

    const toggle = screen.getByRole("button", { name: "Показать таблицей" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    await user.click(toggle);

    const table = screen.getByRole("table", { name: /^График сессий/ });
    const rows = within(table).getAllByRole("row");
    expect(rows.map((r) => r.textContent?.replace(/\u00a0/g, " "))).toEqual([
      "ПериодСессииПредыдущий период",
      "12 окт, 00:00108",
      "12 окт, 01:001 204900",
    ]);
    expect(screen.getByRole("button", { name: "Скрыть таблицу" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("formats durations, weeks and months", async () => {
    const user = userEvent.setup();
    const { rerender } = render(
      <MetricChart
        series={{
          metric: "avg_duration_ms",
          granularity: "week",
          points: [
            { ts: "2026-09-28T00:00:00+03:00", value: 252000 },
            { ts: "2026-10-05T00:00:00+03:00", value: null },
          ],
        }}
        granularity="week"
        metric="avg_duration_ms"
        timezone="Europe/Moscow"
      />,
    );
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^График средней длительности сессии по неделям, 28\sсен – 5\sокт\s2026, максимум 4\sмин\s12\sс 28\sсен$/,
    );
    await user.click(screen.getByRole("button", { name: "Показать таблицей" }));
    expect(screen.getAllByRole("row")[2]).toHaveTextContent(/^5\sокт—$/);

    rerender(
      <MetricChart
        series={{
          metric: "new",
          granularity: "month",
          points: [
            { ts: "2025-12-01T00:00:00+03:00", value: 5 },
            { ts: "2026-01-01T00:00:00+03:00", value: 7 },
          ],
        }}
        granularity="month"
        metric="new"
        timezone="Europe/Moscow"
      />,
    );
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^График новых пользователей по месяцам, 1\sдек\s2025 – 1\sянв\s2026, максимум 7 янв\s2026$/,
    );
  });

  it("shows a flat line with a note when the period has no events", () => {
    render(
      <MetricChart
        series={{ ...daily, points: daily.points.map((p) => ({ ...p, value: 0 })) }}
        granularity="day"
        metric="active"
        timezone="Europe/Moscow"
      />,
    );
    expect(screen.getByText("Нет событий за выбранный период")).toBeVisible();
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^График активных пользователей по дням, 6–12\sокт\s2026, нет событий$/,
    );
  });
});
