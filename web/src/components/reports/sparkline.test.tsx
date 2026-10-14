import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Sparkline } from "./sparkline";

// Точки линии из атрибута d: "M x y L x y …".
function points(container: HTMLElement): [number, number][] {
  const d = container.querySelector("[data-sparkline] path")?.getAttribute("d") ?? "";
  return [...d.matchAll(/[ML]\s*(-?[\d.]+)[ ,](-?[\d.]+)/g)].map((m) => [
    Number(m[1]),
    Number(m[2]),
  ]);
}

describe("Sparkline", () => {
  it("draws the trend as a decorative inline SVG with the last point marked", () => {
    const { container } = render(
      <Sparkline
        values={[1, 3, 2, 5]}
        width={96}
        height={28}
        trendLabel="Тренд за 30 дней: рост"
      />,
    );
    const svg = container.querySelector("[data-sparkline] svg");
    expect(svg).toHaveAttribute("width", "96");
    expect(svg).toHaveAttribute("height", "28");
    expect(container.querySelector("[data-sparkline]")).toHaveAttribute("aria-hidden", "true");
    expect(svg?.querySelector("[tabindex]")).toBeNull();
    expect(screen.getByText("Тренд за 30 дней: рост")).toHaveClass("sr-only");

    const line = points(container);
    expect(line).toHaveLength(4);
    // Слева направо с отступом 4 px; больше значение — выше точка.
    expect(line[0]?.[0]).toBe(4);
    expect(line[3]?.[0]).toBe(92);
    expect(line[3]?.[1]).toBe(4);
    expect(line[0]?.[1]).toBe(24);
    expect(line[1]?.[1]).toBeLessThan(line[2]?.[1] ?? 0);

    const dot = container.querySelector("[data-sparkline] circle");
    expect(dot).toHaveAttribute("r", "3");
    expect(dot).toHaveAttribute("cx", "92");
    expect(dot).toHaveAttribute("cy", "4");
  });

  it("draws a flat line through the middle when all values are equal", () => {
    const { container } = render(
      <Sparkline values={[0, 0, 0]} width={96} height={28} trendLabel="Тренд: без изменений" />,
    );
    expect(points(container).map(([, y]) => y)).toEqual([14, 14, 14]);
  });

  it("marks a single value without a line", () => {
    const { container } = render(
      <Sparkline values={[7]} width={96} height={28} trendLabel="Тренд: без изменений" />,
    );
    expect(points(container)).toHaveLength(1);
    expect(container.querySelector("[data-sparkline] circle")).toHaveAttribute("cx", "92");
  });

  it("renders only the label when there are no values", () => {
    const { container } = render(<Sparkline values={[]} trendLabel="Нет данных" />);
    expect(container.querySelector("[data-sparkline] path")).toBeNull();
    expect(container.querySelector("[data-sparkline] circle")).toBeNull();
    expect(screen.getByText("Нет данных")).toBeInTheDocument();
  });
});
