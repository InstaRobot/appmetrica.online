import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { EnvironmentSwitch } from "./environment-switch";
import { GranularitySelect } from "./granularity-select";
import { PeriodPicker } from "./period-picker";

// Макет «Dashboard v2»: переключатели отчётов — сегментированный контрол (серая подложка, активный
// пункт — приподнятая белая «таблетка»), недоступные варианты видны, но приглушены.
describe("report segmented controls", () => {
  it.each([
    [
      "Период",
      <PeriodPicker
        key="p"
        value={{ preset: "7d", from: "2026-10-07", to: "2026-10-13" }}
        onChange={vi.fn()}
        timezone="Europe/Moscow"
      />,
    ],
    ["Окружение", <EnvironmentSwitch key="e" value="production" onChange={vi.fn()} />],
    [
      "Детализация",
      <GranularitySelect key="g" value="day" allowed={["day", "week"]} onChange={vi.fn()} />,
    ],
  ])("%s uses the segmented variant", (name, element) => {
    render(element);

    const group = screen.getByRole("radiogroup", { name });
    expect(group).toHaveAttribute("data-variant", "segmented");
    for (const item of screen.getAllByRole("radio")) {
      expect(item).toHaveAttribute("data-variant", "segmented");
    }
  });
});
