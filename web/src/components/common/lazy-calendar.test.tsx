import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { LazyCalendar } from "./lazy-calendar";

const OCTOBER = new Date(2026, 9, 1);

// Календарь грузится отдельным чанком: до него — заглушка с подписью, после — месяцы на языке интерфейса.
describe("LazyCalendar", () => {
  it("speaks English", async () => {
    renderWithLocale(<LazyCalendar mode="single" month={OCTOBER} />, "en");

    expect(screen.getByRole("status", { name: "Loading calendar…" })).toBeInTheDocument();
    // Первая загрузка чанка с react-day-picker в jsdom дольше секунды по умолчанию.
    expect(await screen.findByText("October 2026", {}, { timeout: 5000 })).toBeInTheDocument();
  });

  it("speaks Russian", async () => {
    render(<LazyCalendar mode="single" month={OCTOBER} />);

    // Чанк уже загружен первым тестом: заглушки нет, сразу календарь.
    expect(await screen.findByText(/^октябрь 2026$/i)).toBeInTheDocument();
  });
});
