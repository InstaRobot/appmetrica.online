import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Delta } from "./delta";
import { Sparkline } from "./sparkline";

const NBSP = " ";

describe("Delta", () => {
  it.each([
    [4.1, `+4,1${NBSP}%`, "text-positive"],
    [-1.2, `−1,2${NBSP}%`, "text-negative"],
    [0, `0${NBSP}%`, "text-fg-muted"],
  ])("renders %d with sign and tone", (value, text, tone) => {
    render(<Delta value={value} />);
    expect(screen.getByText(text)).toHaveClass(tone);
  });

  it("renders a missing comparison as a dash", () => {
    render(<Delta value={null} />);
    expect(screen.getByText("—")).toHaveClass("text-fg-muted");
  });

  it("inverts tones for metrics where growth is bad", () => {
    render(<Delta value={2.5} invert />);
    expect(screen.getByText(`+2,5${NBSP}%`)).toHaveClass("text-negative");
  });
});

describe("Sparkline", () => {
  it("hides the chart from assistive technology and keeps the trend label", () => {
    const { container } = render(
      <Sparkline values={[1, 2, 3, 4]} trendLabel="Тренд за 30 дней: рост" />,
    );
    expect(screen.getByText("Тренд за 30 дней: рост")).toHaveClass("sr-only");
    const chart = container.querySelector("[data-sparkline]");
    expect(chart).toHaveAttribute("aria-hidden", "true");
    expect(chart).toHaveStyle({ width: "96px", height: "28px" });
  });
});
