import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { CHART_HEIGHT } from "./responsive";
import { RetentionAverageChart } from "./retention-average-chart";

const plain = (text: string | null) => (text ?? "").replace(/ /g, " ");

const dayAverage = Array.from({ length: 31 }, (_, n) => (n === 0 ? 1 : n > 28 ? null : 0.4 / n));

describe("RetentionAverageChart", () => {
  it("describes D1, D7 and D30 and lists them", () => {
    render(<RetentionAverageChart average={dayAverage} granularity="day" />);

    expect(plain(screen.getByRole("img").getAttribute("aria-label"))).toBe(
      "Средняя кривая удержания по дням: D1 40 %, D7 5,7 %, D30 нет данных",
    );
    const keys = screen.getByRole("list", { name: "Ключевые точки удержания" });
    expect(
      within(keys)
        .getAllByRole("listitem")
        .map((li) => plain(li.textContent)),
    ).toEqual(["D140 %", "D75,7 %", "D30—"]);
  });

  it("uses W1, W4 and W12 for week cohorts and skips points beyond the matrix", () => {
    render(<RetentionAverageChart average={[1, 0.3, 0.2, 0.15, 0.12]} granularity="week" />);

    expect(plain(screen.getByRole("img").getAttribute("aria-label"))).toBe(
      "Средняя кривая удержания по неделям: W1 30 %, W4 12 %",
    );
  });

  it("wraps the key points and caps the chart height on a phone", () => {
    render(<RetentionAverageChart average={dayAverage} granularity="day" />);

    expect(screen.getByRole("list", { name: "Ключевые точки удержания" })).toHaveClass("flex-wrap");
    const chart = screen.getByRole("img");
    expect(chart).toHaveClass(CHART_HEIGHT);
    expect(chart.style.getPropertyValue("--chart-h")).toBe("220px");
  });

  it("shows the curve as a table on demand", async () => {
    const user = userEvent.setup();
    render(<RetentionAverageChart average={[1, 0.381, null]} granularity="day" />);

    await user.click(screen.getByRole("button", { name: "Показать таблицей" }));
    const table = screen.getByRole("table");
    const rows = within(table)
      .getAllByRole("row")
      .map((r) => plain(r.textContent));
    expect(rows).toEqual(["ДеньСреднее удержание", "D0100 %", "D138,1 %", "D2—"]);
  });

  it("renders a placeholder when no period has data", () => {
    render(<RetentionAverageChart average={[null, null]} granularity="day" />);

    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByText("Нет данных за выбранный период")).toBeInTheDocument();
  });
});

describe("RetentionAverageChart in English", () => {
  it("describes and lists the key points in English", async () => {
    const user = userEvent.setup();
    renderWithLocale(<RetentionAverageChart average={dayAverage} granularity="day" />, "en");

    expect(screen.getByRole("img")).toHaveAccessibleName(
      "Average retention curve by day: D1 40%, D7 5.7%, D30 no data",
    );
    const keys = screen.getByRole("list", { name: "Key retention points" });
    expect(
      within(keys)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual(["D140%", "D75.7%", "D30—"]);

    await user.click(screen.getByRole("button", { name: "Show as table" }));
    expect(within(screen.getByRole("table")).getAllByRole("row")[0]).toHaveTextContent(
      "DayAverage retention",
    );
  });

  it("uses weeks and the placeholder in English", () => {
    const { rerender } = renderWithLocale(
      <RetentionAverageChart average={[1, 0.3, 0.2, 0.15, 0.12]} granularity="week" />,
      "en",
    );
    expect(screen.getByRole("img")).toHaveAccessibleName(
      "Average retention curve by week: W1 30%, W4 12%",
    );

    rerender(<RetentionAverageChart average={[null, null]} granularity="week" />);
    expect(screen.getByText("No data for the selected period")).toBeInTheDocument();
  });
});
