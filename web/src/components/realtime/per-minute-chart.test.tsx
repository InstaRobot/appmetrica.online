import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { RealtimeMinute } from "@/lib/api/gen/model";
import { renderWithLocale } from "@/test/rtl";
import { PerMinuteChart } from "./per-minute-chart";

const points = (values: number[]): RealtimeMinute[] =>
  values.map((sessions_started, i) => ({
    minute: new Date(Date.UTC(2026, 9, 13, 11, 3 + i)).toISOString(),
    sessions_started,
  }));

describe("PerMinuteChart", () => {
  it("describes 30 minutes of session starts in the app time zone", () => {
    const values = Array.from({ length: 30 }, (_, i) => (i === 29 ? 7 : 1));
    render(<PerMinuteChart points={points(values)} timezone="Europe/Moscow" />);

    expect(screen.getByRole("img")).toHaveAccessibleName(
      "Старты сессий по минутам, 14:03–14:32: всего 36, в последнюю минуту 7",
    );
  });

  it("falls back to UTC for an unknown time zone", () => {
    render(<PerMinuteChart points={points([0, 0])} timezone="Nope/Zone" />);
    expect(screen.getByRole("img")).toHaveAccessibleName(
      "Старты сессий по минутам, 11:03–11:04: всего 0, в последнюю минуту 0",
    );
  });

  it("says when there are no points", () => {
    render(<PerMinuteChart points={[]} timezone="Europe/Moscow" />);
    expect(screen.getByText("Нет данных за последние 30 минут")).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });
});

describe("PerMinuteChart in English", () => {
  it("describes session starts in English", () => {
    const values = Array.from({ length: 30 }, (_, i) => (i === 29 ? 1207 : 1));
    renderWithLocale(<PerMinuteChart points={points(values)} timezone="Europe/Moscow" />, "en");

    expect(screen.getByRole("img")).toHaveAccessibleName(
      "Session starts per minute, 14:03–14:32: 1,236 total, 1,207 in the last minute",
    );
  });

  it("says when there are no points in English", () => {
    renderWithLocale(<PerMinuteChart points={[]} timezone="Europe/Moscow" />, "en");
    expect(screen.getByText("No data for the last 30 minutes")).toBeInTheDocument();
  });
});
