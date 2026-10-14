import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { RetentionMatrix } from "@/lib/api/gen/model";
import { RetentionHeatmap } from "./retention-heatmap";

const dayMatrix: RetentionMatrix = {
  cohort_granularity: "day",
  periods: 3,
  average: [1, 0.3605, 0.2412, null],
  cohorts: [
    { cohort_start: "2026-10-09", size: 1310, values: [1, 0.381, 0.25, null] },
    { cohort_start: "2026-10-10", size: 1102, values: [1, 0.3503, 0.0499, null] },
    { cohort_start: "2026-10-11", size: 0, values: [null, null, null, null] },
    { cohort_start: "2026-10-12", size: 1200, values: [1, null, null, null] },
  ],
};

const weekMatrix: RetentionMatrix = {
  cohort_granularity: "week",
  periods: 2,
  average: [1, 0.611, 0.13],
  cohorts: [
    { cohort_start: "2026-09-28", size: 2400, values: [1, 0.62, 0.142] },
    { cohort_start: "2026-10-05", size: 2000, values: [1, 0.6, null] },
  ],
};

// Формат чисел и дат ставит неразрывные пробелы (U+00A0): сравниваем с обычными.
const plain = (text: string | null) => (text ?? "").replace(/\u00a0/g, " ");
// Точное имя с любыми пробелами: «38,1 %» совпадает и с «38,1\u00a0%».
const name = (text: string) =>
  new RegExp(`^${text.replace(/[()]/g, "\\$&").replace(/ /g, "\\s")}$`);

const cell = (row: HTMLElement, col: number) => {
  const node = row.querySelectorAll("td")[col];
  if (!node) throw new Error(`нет ячейки ${col}`);
  return node;
};

describe("RetentionHeatmap", () => {
  it("renders accessible cohort table", () => {
    render(<RetentionHeatmap matrix={dayMatrix} granularity="day" />);

    const table = screen.getByRole("table", {
      name: "Retention по дням: доля когорты, вернувшейся в день N",
    });
    expect(table.querySelector("caption")).not.toBeNull();

    const headers = within(table).getAllByRole("columnheader");
    expect(headers.map((h) => plain(h.textContent))).toEqual([
      "Когорта",
      "Размер",
      "D0",
      "D1",
      "D2",
      "D3",
    ]);
    for (const h of headers) expect(h).toHaveAttribute("scope", "col");

    const rowHeaders = within(table).getAllByRole("rowheader");
    expect(rowHeaders.map((h) => plain(h.textContent))).toEqual([
      "Среднее",
      "9 окт 2026",
      "10 окт 2026",
      "11 окт 2026",
      "12 окт 2026",
    ]);
    for (const h of rowHeaders) expect(h).toHaveAttribute("scope", "row");

    expect(within(table).getByRole("cell", { name: name("38,1 %") })).toBeInTheDocument();
    const empty = within(table).getAllByRole("cell", { name: "нет данных" });
    // D3 у трёх когорт, пустая когорта целиком (4) и D1…D3 вчерашней когорты; среднее D3.
    expect(empty).toHaveLength(1 + 1 + 4 + 3 + 1);
    for (const c of empty) expect(c).toHaveTextContent("");
  });

  it("shows the average row first with mean cohort size", () => {
    render(<RetentionHeatmap matrix={dayMatrix} granularity="day" />);

    const rows = screen.getAllByRole("row");
    // Шапка, затем «Среднее»: средний размер непустых когорт (1310 + 1102 + 1200) / 3.
    const average = rows[1];
    if (!average) throw new Error("нет строки «Среднее»");
    expect(within(average).getByRole("rowheader")).toHaveTextContent("Среднее");
    expect(cell(average, 0)).toHaveTextContent(/^1\s204$/);
    expect(cell(average, 2)).toHaveTextContent(/^36,1\s%$/);
    expect(average).toHaveAttribute("data-average");
  });

  it("colors period cells by the sequential scale and leaves D0 uncolored", () => {
    render(<RetentionHeatmap matrix={dayMatrix} granularity="day" />);

    const first = screen.getByRole("rowheader", { name: name("9 окт 2026") }).closest("tr");
    const second = screen.getByRole("rowheader", { name: name("10 окт 2026") }).closest("tr");
    if (!first || !second) throw new Error("нет строк когорт");

    expect(cell(first, 0)).toHaveTextContent(/^1\s310$/);
    expect(cell(first, 1)).toHaveTextContent(/^100\s%$/);
    expect(cell(first, 1)).not.toHaveAttribute("data-step");
    expect(cell(first, 1)).toHaveClass("bg-subtle", "text-fg-muted");
    expect(cell(first, 2)).toHaveAttribute("data-step", "4");
    expect(cell(first, 2)).toHaveClass("bg-ret-4", "text-white");
    expect(cell(first, 3)).toHaveAttribute("data-step", "3");
    expect(cell(second, 3)).toHaveAttribute("data-step", "0");
    expect(cell(second, 3)).toHaveTextContent(/^5\s%$/);
  });

  it("explains each value with absolute numbers on hover", () => {
    render(<RetentionHeatmap matrix={dayMatrix} granularity="day" unit="persons" />);

    expect(plain(screen.getByRole("cell", { name: name("38,1 %") }).getAttribute("title"))).toBe(
      "Когорта 9 окт 2026 (1 310 чел.) · день 1: 499 чел. (38,1 %)",
    );
  });

  it("labels week cohorts with date ranges and W columns", () => {
    render(<RetentionHeatmap matrix={weekMatrix} granularity="week" unit="installs" />);

    const table = screen.getByRole("table", {
      name: "Retention по неделям: доля когорты, вернувшейся в неделю N",
    });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((h) => plain(h.textContent)),
    ).toEqual(["Когорта", "Размер", "W0", "W1", "W2"]);
    expect(
      within(table).getByRole("rowheader", { name: name("28 сен – 4 окт") }),
    ).toBeInTheDocument();
    expect(within(table).getByRole("rowheader", { name: name("5–11 окт") })).toBeInTheDocument();
    expect(within(table).getByRole("cell", { name: name("62 %") })).toHaveAttribute(
      "data-step",
      "5",
    );
    expect(
      plain(
        within(table)
          .getByRole("cell", { name: name("14,2 %") })
          .getAttribute("title"),
      ),
    ).toBe("Когорта 28 сен – 4 окт (2 400 устр.) · неделя 2: 341 устр. (14,2 %)");
  });

  it("highlights a period column when its header is clicked", async () => {
    const user = userEvent.setup();
    render(<RetentionHeatmap matrix={dayMatrix} granularity="day" />);

    const d1 = screen.getByRole("button", { name: "D1" });
    expect(d1).toHaveAttribute("aria-pressed", "false");
    await user.click(d1);
    expect(d1).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("cell", { name: name("38,1 %") })).toHaveAttribute("data-selected");
    expect(screen.getByRole("cell", { name: name("25 %") })).not.toHaveAttribute("data-selected");

    await user.click(d1);
    expect(d1).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("cell", { name: name("38,1 %") })).not.toHaveAttribute("data-selected");
  });

  it("keeps cohort and size columns sticky inside a focusable scroll region", () => {
    render(<RetentionHeatmap matrix={dayMatrix} granularity="day" />);

    const region = screen.getByRole("region", {
      name: "Матрица когорт, прокручивается по горизонтали",
    });
    expect(region).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("columnheader", { name: "Когорта" })).toHaveClass("sticky", "left-0");
    expect(screen.getByRole("columnheader", { name: "Размер" })).toHaveClass(
      "sticky",
      "left-[132px]",
    );
    expect(screen.getByRole("rowheader", { name: name("9 окт 2026") })).toHaveClass(
      "sticky",
      "left-0",
    );
  });
});
