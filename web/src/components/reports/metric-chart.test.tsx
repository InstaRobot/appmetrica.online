import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { TimeSeries } from "@/lib/api/gen/model";
import { renderWithLocale } from "@/test/rtl";
import { MetricChart as LazyMetricChart } from "./lazy-charts";
import { MetricChart, type MetricChartSeries } from "./metric-chart";
import { CHART_HEIGHT } from "./responsive";

const day = (d: number) => `2026-10-${String(d).padStart(2, "0")}T00:00:00+03:00`;

const daily: TimeSeries = {
  metric: "active",
  granularity: "day",
  points: [6, 7, 8, 9, 10, 11, 12].map((d) => ({ ts: day(d), value: d === 7 ? 12480 : 9000 + d })),
};

// Итог — единственная серия с рядом предыдущего периода, как в режиме «Итог» аудитории.
const total = (ts: TimeSeries): MetricChartSeries[] => [
  {
    key: "total",
    label: "Итог",
    points: ts.points,
    ...(ts.compare_points ? { compare: ts.compare_points } : {}),
  },
];

describe("MetricChart", () => {
  it("describes the chart for screen readers", () => {
    render(
      <MetricChart
        series={total(daily)}
        granularity="day"
        metric="active"
        timezone="Europe/Moscow"
      />,
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
      <MetricChart
        series={total(series)}
        granularity="hour"
        metric="sessions"
        timezone="Europe/Moscow"
      />,
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
        series={total({
          metric: "avg_duration_ms",
          granularity: "week",
          points: [
            { ts: "2026-09-28T00:00:00+03:00", value: 252000 },
            { ts: "2026-10-05T00:00:00+03:00", value: null },
          ],
        })}
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
        series={total({
          metric: "new",
          granularity: "month",
          points: [
            { ts: "2025-12-01T00:00:00+03:00", value: 5 },
            { ts: "2026-01-01T00:00:00+03:00", value: 7 },
          ],
        })}
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
        series={total({ ...daily, points: daily.points.map((p) => ({ ...p, value: 0 })) })}
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

  it("renders a line per platform", async () => {
    const user = userEvent.setup();
    render(
      <MetricChart
        series={[
          {
            key: "ios",
            label: "iOS",
            points: [
              { ts: day(6), value: 5210 },
              { ts: day(7), value: 5400 },
            ],
          },
          {
            key: "android",
            label: "Android",
            points: [
              { ts: day(6), value: 3870 },
              { ts: day(7), value: 3605 },
            ],
          },
        ]}
        granularity="day"
        metric="active"
        timezone="Europe/Moscow"
      />,
    );

    const legend = screen.getByRole("list", { name: "Легенда графика" });
    expect(
      within(legend)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual(["iOS", "Android"]);
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^График активных пользователей по дням по платформам, 6–7\sокт\s2026, iOS: максимум 5\s400 7\sокт, Android: максимум 3\s870 6\sокт$/,
    );

    await user.click(screen.getByRole("button", { name: "Показать таблицей" }));
    const rows = within(screen.getByRole("table")).getAllByRole("row");
    expect(rows.map((r) => r.textContent?.replace(/\u00a0/g, " "))).toEqual([
      "ПериодiOSAndroid",
      "6 окт5 2103 870",
      "7 окт5 4003 605",
    ]);
    expect(within(rows[0] as HTMLElement).getAllByRole("columnheader")).toHaveLength(3);
  });

  it("keeps platforms without events and an empty split in the table", async () => {
    const user = userEvent.setup();
    const { rerender } = render(
      <MetricChart
        series={[
          { key: "web", label: "Веб", points: [{ ts: day(6), value: 0 }] },
          { key: "tvos", label: "tvos", points: [{ ts: day(6), value: null }] },
        ]}
        granularity="day"
        metric="new"
        timezone="Europe/Moscow"
      />,
    );
    expect(screen.getByText("Нет событий за выбранный период")).toBeVisible();
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^График новых пользователей по дням по платформам, 6\sокт\s2026, нет событий$/,
    );
    await user.click(screen.getByRole("button", { name: "Показать таблицей" }));
    expect(screen.getAllByRole("row").map((r) => r.textContent)).toEqual([
      "ПериодВебtvos",
      expect.stringMatching(/^6\sокт0—$/),
    ]);

    rerender(
      <MetricChart series={[]} granularity="day" metric="active" timezone="Europe/Moscow" />,
    );
    expect(screen.getByText("Нет событий за выбранный период")).toBeVisible();
    expect(screen.getByRole("img")).toHaveAccessibleName(
      "График активных пользователей по дням, нет событий",
    );
  });
});

describe("MetricChart in English", () => {
  it("describes the chart and the table in English", async () => {
    const user = userEvent.setup();
    renderWithLocale(
      <MetricChart
        series={total({
          ...daily,
          compare_points: daily.points.map((p) => ({ ...p, value: 9000 })),
        })}
        granularity="day"
        metric="active"
        timezone="Europe/Moscow"
      />,
      "en",
    );
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^Active users chart by day, Oct\s6–12, 2026, peak 12,480 \(Oct\s7\)$/,
    );

    await user.click(screen.getByRole("button", { name: "Show as table" }));
    const rows = within(screen.getByRole("table")).getAllByRole("row");
    expect(rows.slice(0, 3).map((r) => r.textContent?.replace(/\u00a0/g, " "))).toEqual([
      "PeriodActive usersPrevious period",
      "Oct 69,0069,000",
      "Oct 712,4809,000",
    ]);
    expect(screen.getByRole("button", { name: "Hide table" })).toBeInTheDocument();
  });

  it("describes durations by hour and an empty period in English", () => {
    const { rerender } = renderWithLocale(
      <MetricChart
        series={total({
          metric: "avg_duration_ms",
          granularity: "hour",
          points: [
            { ts: "2026-10-12T00:00:00+03:00", value: 60000 },
            { ts: "2026-10-12T01:00:00+03:00", value: 252000 },
          ],
        })}
        granularity="hour"
        metric="avg_duration_ms"
        timezone="Europe/Moscow"
      />,
      "en",
    );
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^Average session duration chart by hour, Oct\s12, 2026, peak 4m 12s \(Oct\s12, 01:00\)$/,
    );

    rerender(
      <MetricChart series={[]} granularity="week" metric="sessions" timezone="Europe/Moscow" />,
    );
    expect(screen.getByText("No events in the selected period")).toBeVisible();
    expect(screen.getByRole("img")).toHaveAccessibleName("Sessions chart by week, no events");
  });

  it("names the platform lines in English", () => {
    renderWithLocale(
      <MetricChart
        series={[
          { key: "ios", label: "iOS", points: [{ ts: day(6), value: 5210 }] },
          { key: "android", label: "Android", points: [{ ts: day(6), value: 3870 }] },
        ]}
        granularity="month"
        metric="new"
        timezone="Europe/Moscow"
      />,
      "en",
    );
    expect(screen.getByRole("list", { name: "Chart legend" })).toBeInTheDocument();
    expect(screen.getByRole("img")).toHaveAccessibleName(
      /^New users chart by month per platform, Oct\s6, 2026, iOS: peak 5,210 \(Oct\s2026\), Android: peak 3,870 \(Oct\s2026\)$/,
    );
  });

  // jsdom не считает CSS: высоту на телефоне задают брейкпоинт-классы и переменная --chart-h.
  it("is at most 220 px tall on a phone and keeps its height from md", () => {
    render(
      <MetricChart
        series={total(daily)}
        granularity="day"
        metric="active"
        timezone="Europe/Moscow"
      />,
    );
    const chart = screen.getByRole("img");
    expect(chart).toHaveClass(CHART_HEIGHT);
    expect(chart.style.getPropertyValue("--chart-h")).toBe("360px");
    expect(chart.style.height).toBe("");
  });

  it("announces the chart while its code loads", async () => {
    renderWithLocale(
      <LazyMetricChart
        series={total(daily)}
        granularity="day"
        metric="active"
        timezone="Europe/Moscow"
      />,
      "en",
    );
    // Заглушка высотой с график и «Показать таблицей», на телефоне — тоже не выше 220 px графика.
    const placeholder = screen.getByRole("status", { name: "Loading chart…" });
    expect(placeholder.style.getPropertyValue("--chart-h")).toBe("360px");
    expect(placeholder.style.getPropertyValue("--chart-extra")).toBe("40px");
    expect(placeholder).toHaveClass(
      "h-[calc(min(var(--chart-h),220px)+var(--chart-extra))]",
      "md:h-[calc(var(--chart-h)+var(--chart-extra))]",
    );
    expect(await screen.findByRole("img", { name: /^Active users chart/ })).toBeInTheDocument();
  });
});
