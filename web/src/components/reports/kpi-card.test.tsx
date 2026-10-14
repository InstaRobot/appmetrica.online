import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { deltaLabel, KpiCard } from "./kpi-card";

const days = (values: number[]) =>
  values.map((value, i) => ({ ts: `2026-10-0${i + 1}T00:00:00+03:00`, value }));

describe("KpiCard", () => {
  it("shows the value, delta and sparkline", () => {
    const { container } = render(
      <KpiCard
        label="Активные"
        definition="Уникальные люди"
        value={41230}
        format="number"
        deltaPct={6.2}
        sparkline={days([1, 2, 3])}
      />,
    );
    const card = screen.getByRole("button", { name: /^Активные/ });
    expect(card).toHaveTextContent(/41\s230/);
    expect(
      screen.getByRole("img", { name: /^рост на 6,2\s% к предыдущему периоду$/ }),
    ).toBeVisible();
    expect(container.querySelector("[data-sparkline]")).not.toBeNull();
    expect(screen.getByText("Тренд за период: рост")).toBeInTheDocument();
  });

  it("formats durations and shows the secondary line", () => {
    render(
      <KpiCard
        label="Ср. длительность"
        definition="…"
        value={252000}
        format="duration"
        secondary="медиана 2 мин 5 с"
      />,
    );
    expect(screen.getByRole("button", { pressed: false })).toHaveTextContent(/4\sмин\s12\sс/);
    expect(screen.getByText("медиана 2 мин 5 с")).toBeInTheDocument();
  });

  it("renders missing values and deltas neutrally", () => {
    render(
      <KpiCard
        label="Ср. длительность"
        definition="…"
        value={null}
        format="duration"
        deltaPct={null}
      />,
    );
    expect(screen.getByRole("button", { pressed: false })).toHaveTextContent("—");
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.queryByText(/Тренд/)).not.toBeInTheDocument();
  });

  it("is a toggle that selects the chart metric", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    const { rerender } = render(
      <KpiCard label="Новые" definition="…" value={10} format="number" onSelect={onSelect} />,
    );
    const card = screen.getByRole("button", { name: /^Новые/ });
    expect(card).toHaveAttribute("aria-pressed", "false");
    await user.click(card);
    expect(onSelect).toHaveBeenCalledOnce();

    rerender(<KpiCard label="Новые" definition="…" value={10} format="number" selected />);
    expect(screen.getByRole("button", { name: /^Новые/ })).toHaveAttribute("aria-pressed", "true");
  });

  it("explains the metric in a tooltip", async () => {
    const user = userEvent.setup();
    render(
      <KpiCard
        label="Сессии"
        definition="Число уникальных session_id."
        value={1}
        format="number"
      />,
    );
    await user.hover(screen.getByRole("button", { name: "Что такое «Сессии»" }));
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Число уникальных session_id.");
  });

  it("shows a skeleton while loading", () => {
    render(<KpiCard label="Активные" definition="…" value={null} format="number" loading />);
    expect(screen.getByRole("status", { name: "Загрузка…" })).toBeInTheDocument();
    expect(screen.queryByText("—")).not.toBeInTheDocument();
  });
});

describe("deltaLabel", () => {
  it("describes growth, decline and no change in words", () => {
    expect(deltaLabel(4.1)).toBe("рост на 4,1 % к предыдущему периоду");
    expect(deltaLabel(-1.24)).toBe("снижение на 1,2 % к предыдущему периоду");
    expect(deltaLabel(0.04)).toBe("без изменений к предыдущему периоду");
    expect(deltaLabel(null)).toBeNull();
  });
});
