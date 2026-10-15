import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { CompareToggle } from "./compare-toggle";
import { GranularitySelect } from "./granularity-select";
import { PeriodPicker } from "./period-picker";
import { STRETCH_GROUP, STRETCH_ITEM, TOUCH_BUTTON, TOUCH_TOGGLE } from "./responsive";

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

    // Код popover грузится лениво: ждём настоящий триггер (с aria-expanded).
    await user.click(await screen.findByRole("button", { name: "Выбрать даты", expanded: false }));
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

// jsdom не считает CSS: телефонная вёрстка проверяется по брейкпоинт-классам.
describe("report controls on a phone", () => {
  it("keeps presets in one stretched row with 44 px targets next to the calendar", () => {
    render(
      <PeriodPicker
        value={{ preset: "7d", from: "2026-10-07", to: "2026-10-13" }}
        onChange={vi.fn()}
        timezone="Europe/Moscow"
      />,
    );

    expect(screen.getByRole("radiogroup", { name: "Период" })).toHaveClass(
      "flex-1",
      "md:flex-none",
    );
    for (const radio of screen.getAllByRole("radio")) {
      expect(radio).toHaveClass(TOUCH_TOGGLE, "flex-auto", "md:flex-none");
    }
    expect(screen.getByRole("button", { name: "Выбрать даты" })).toHaveClass(
      TOUCH_BUTTON,
      "min-w-11",
      "md:min-w-0",
    );
  });

  it("stretches granularity over the phone width and makes the compare label tall", () => {
    render(
      <>
        <GranularitySelect value="day" allowed={["hour", "day"]} onChange={vi.fn()} />
        <CompareToggle checked={false} onCheckedChange={vi.fn()} />
      </>,
    );

    expect(screen.getByRole("radiogroup", { name: "Детализация" })).toHaveClass(STRETCH_GROUP);
    for (const radio of screen.getAllByRole("radio")) {
      expect(radio).toHaveClass(TOUCH_TOGGLE, STRETCH_ITEM);
    }
    expect(screen.getByText("Сравнить")).toHaveClass("h-11", "md:h-auto");
  });
});

describe("report controls in English", () => {
  it("names the presets and a custom range in English", () => {
    const { rerender } = renderWithLocale(
      <PeriodPicker
        value={{ preset: "7d", from: "2026-10-07", to: "2026-10-13" }}
        onChange={vi.fn()}
        timezone="Europe/Moscow"
      />,
      "en",
    );
    expect(screen.getByRole("radiogroup", { name: "Period" })).toHaveTextContent(
      "TodayYesterday7 days30 days90 days",
    );
    expect(screen.getByRole("button", { name: "Pick dates" })).toBeInTheDocument();

    rerender(
      <PeriodPicker
        value={{ preset: "custom", from: "2026-09-28", to: "2026-10-05" }}
        onChange={vi.fn()}
        timezone="Europe/Moscow"
      />,
    );
    expect(
      screen.getByRole("button", { name: /^Pick dates: Sep\s28 – Oct\s5, 2026$/ }),
    ).toBeVisible();
  });

  it("names granularities and comparison in English", () => {
    renderWithLocale(
      <>
        <GranularitySelect value="day" allowed={["hour", "day"]} onChange={vi.fn()} />
        <CompareToggle checked={false} onCheckedChange={vi.fn()} />
        <CompareToggle checked={false} onCheckedChange={vi.fn()} disabledReason="Not now" />
      </>,
      "en",
    );
    expect(screen.getByRole("radiogroup", { name: "Granularity" })).toHaveTextContent(
      "HourDayWeekMonth",
    );
    expect(screen.getByRole("radio", { name: "Day" })).toHaveAttribute("aria-checked", "true");
    const switches = screen.getAllByRole("switch", { name: "Compare with the previous period" });
    expect(switches).toHaveLength(2);
    expect(switches[1]).toHaveAccessibleDescription("Not now");
    expect(screen.getAllByText("Compare")).toHaveLength(2);
  });
});
