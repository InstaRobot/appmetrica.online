import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { createFormatters } from "@/lib/format";
import { getMessages } from "@/messages";
import { renderWithLocale } from "@/test/rtl";
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

describe("KpiCard in English", () => {
  it("formats the value, delta and trend in English", async () => {
    const user = userEvent.setup();
    renderWithLocale(
      <KpiCard
        label="Active users"
        definition="People with at least one session."
        value={41230}
        format="number"
        deltaPct={-3}
        secondary="median 2m 5s"
        sparkline={days([3, 2, 1])}
      />,
      "en",
    );
    const card = screen.getByRole("button", { name: /^Active users/ });
    expect(card).toHaveTextContent("41,230");
    expect(screen.getByRole("img", { name: "down 3% vs. the previous period" })).toBeVisible();
    expect(screen.getByText("Trend for the period: down")).toBeInTheDocument();

    await user.hover(screen.getByRole("button", { name: "What is “Active users”?" }));
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "People with at least one session.",
    );
  });

  it("formats durations and announces loading in English", () => {
    const { rerender } = renderWithLocale(
      <KpiCard label="Avg. duration" definition="…" value={252000} format="duration" />,
      "en",
    );
    expect(screen.getByRole("button", { pressed: false })).toHaveTextContent("4m 12s");

    rerender(
      <KpiCard label="Avg. duration" definition="…" value={null} format="duration" loading />,
    );
    expect(screen.getByRole("status", { name: "Loading…" })).toBeInTheDocument();
  });
});

describe("deltaLabel", () => {
  const ru = getMessages("ru").reports.kpi.delta;
  const en = getMessages("en").reports.kpi.delta;

  it("describes growth, decline and no change in words", () => {
    const f = createFormatters("ru");
    expect(deltaLabel(4.1, ru, f)).toBe("рост на 4,1 % к предыдущему периоду");
    expect(deltaLabel(-1.24, ru, f)).toBe("снижение на 1,2 % к предыдущему периоду");
    expect(deltaLabel(0.04, ru, f)).toBe("без изменений к предыдущему периоду");
    expect(deltaLabel(null, ru, f)).toBeNull();
  });

  it("describes the change in English", () => {
    const f = createFormatters("en");
    expect(deltaLabel(4.1, en, f)).toBe("up 4.1% vs. the previous period");
    expect(deltaLabel(-1.24, en, f)).toBe("down 1.2% vs. the previous period");
    expect(deltaLabel(0.04, en, f)).toBe("no change vs. the previous period");
  });
});
