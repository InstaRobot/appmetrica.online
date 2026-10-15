import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { IntegrationHour } from "@/lib/api/gen/model";
import { renderWithLocale } from "@/test/rtl";
import { HourlyIngestChart } from "./hourly-ingest-chart";

const NBSP = "\u00a0";

const hours = (n: number): IntegrationHour[] =>
  Array.from({ length: n }, (_, i) => ({
    hour: new Date(Date.UTC(2026, 9, 12, 12 + i)).toISOString(),
    accepted: 1000 + i,
    rejected: i % 2,
  }));

describe("HourlyIngestChart", () => {
  it("describes accepted and rejected events by hour", () => {
    render(<HourlyIngestChart hourly={hours(24)} timezone="Europe/Moscow" />);
    expect(screen.getByRole("img")).toHaveAccessibleName(
      `События по часам, 15:00–14:00: принято 24${NBSP}276, отклонено 12`,
    );
  });

  it("says when there are no hours", () => {
    render(<HourlyIngestChart hourly={[]} timezone="Europe/Moscow" />);
    expect(screen.getByText("Нет данных за 24 часа")).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });

  it("describes the chart in English", () => {
    const { unmount } = renderWithLocale(
      <HourlyIngestChart hourly={hours(24)} timezone="Europe/Moscow" />,
      "en",
    );
    expect(screen.getByRole("img")).toHaveAccessibleName(
      "Events by hour, 15:00–14:00: 24,276 accepted, 12 rejected",
    );
    unmount();

    renderWithLocale(<HourlyIngestChart hourly={[]} timezone="Europe/Moscow" />, "en");
    expect(screen.getByText("No data for the last 24 hours")).toBeInTheDocument();
  });
});
