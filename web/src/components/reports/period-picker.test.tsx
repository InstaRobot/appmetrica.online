import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CompareToggle } from "./compare-toggle";
import { GranularitySelect } from "./granularity-select";
import { PeriodPicker } from "./period-picker";

beforeEach(() => {
  // 2026-10-13 01:30 по Москве.
  vi.useFakeTimers({ now: new Date("2026-10-12T22:30:00Z"), shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
});

describe("PeriodPicker", () => {
  it("switches presets in the app time zone", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <PeriodPicker
        value={{ preset: "7d", from: "2026-10-07", to: "2026-10-13" }}
        onChange={onChange}
        timezone="Europe/Moscow"
      />,
    );

    const group = screen.getByRole("radiogroup", { name: "Период" });
    expect(screen.getByRole("radio", { name: "7 дней" })).toHaveAttribute("aria-checked", "true");
    expect(group).toHaveTextContent("СегодняВчера7 дней30 дней90 дней");

    await user.click(screen.getByRole("radio", { name: "30 дней" }));
    expect(onChange).toHaveBeenLastCalledWith({
      preset: "30d",
      from: "2026-09-14",
      to: "2026-10-13",
    });
    await user.click(screen.getByRole("radio", { name: "Вчера" }));
    expect(onChange).toHaveBeenLastCalledWith({
      preset: "yesterday",
      from: "2026-10-12",
      to: "2026-10-12",
    });
    // Повторный клик по выбранному пресету ничего не меняет.
    onChange.mockClear();
    await user.click(screen.getByRole("radio", { name: "7 дней" }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("picks a custom range in the calendar", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <PeriodPicker
        value={{ preset: "7d", from: "2026-10-07", to: "2026-10-13" }}
        onChange={onChange}
        timezone="Europe/Moscow"
      />,
    );

    await user.click(screen.getByRole("button", { name: "Выбрать даты" }));
    const apply = screen.getByRole("button", { name: "Применить" });
    expect(apply).toBeDisabled();
    await user.click(await screen.findByRole("button", { name: /(^|\s)1 октября 2026/ }));
    await user.click(screen.getByRole("button", { name: /(^|\s)5 октября 2026/ }));
    await user.click(apply);

    expect(onChange).toHaveBeenCalledWith({
      preset: "custom",
      from: "2026-10-01",
      to: "2026-10-05",
    });
    // Дни после сегодняшнего недоступны.
    expect(screen.queryByRole("button", { name: "Применить" })).not.toBeInTheDocument();
  });

  it("labels a custom range and shows the range error", () => {
    render(
      <PeriodPicker
        value={{ preset: "custom", from: "2026-10-06", to: "2026-10-12" }}
        onChange={vi.fn()}
        timezone="Europe/Moscow"
        error="Слишком большой период"
      />,
    );
    expect(screen.getByRole("button", { name: /^Выбрать даты: 6–12\sокт\s2026$/ })).toBeVisible();
    expect(screen.queryByRole("radio", { checked: true })).not.toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("Слишком большой период");
  });

  it("offers only the given presets", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <PeriodPicker
        value={{ preset: "custom", from: "2026-09-13", to: "2026-10-12" }}
        onChange={onChange}
        timezone="Europe/Moscow"
        presets={["7d", "30d", "90d"]}
      />,
    );

    expect(screen.getByRole("radiogroup", { name: "Период" })).toHaveTextContent(
      "7 дней30 дней90 дней",
    );
    expect(screen.queryByRole("radio", { name: "Сегодня" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: "90 дней" }));
    expect(onChange).toHaveBeenLastCalledWith({
      preset: "90d",
      from: "2026-07-16",
      to: "2026-10-13",
    });
  });
});

describe("GranularitySelect", () => {
  it("offers only allowed granularities", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<GranularitySelect value="day" allowed={["hour", "day"]} onChange={onChange} />);

    expect(screen.getByRole("radiogroup", { name: "Детализация" })).toHaveTextContent(
      "часденьнеделямесяц",
    );
    expect(screen.getByRole("radio", { name: "день" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("radio", { name: "неделя" })).toBeDisabled();
    expect(screen.getByRole("radio", { name: "месяц" })).toBeDisabled();
    await user.click(screen.getByRole("radio", { name: "час" }));
    expect(onChange).toHaveBeenCalledWith("hour");
  });
});

describe("CompareToggle", () => {
  it("toggles comparison with the previous period", async () => {
    const onCheckedChange = vi.fn();
    const user = userEvent.setup();
    render(<CompareToggle checked={false} onCheckedChange={onCheckedChange} />);
    await user.click(screen.getByRole("switch", { name: "Сравнить с предыдущим периодом" }));
    expect(onCheckedChange).toHaveBeenCalledWith(true);
  });
});
