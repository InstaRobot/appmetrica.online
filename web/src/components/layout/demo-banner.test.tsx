import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { DemoBanner } from "./demo-banner";

describe("DemoBanner", () => {
  it("explains the synthetic data and offers a way out", () => {
    render(<DemoBanner locale="ru" />);

    const banner = screen.getByRole("region", { name: "Демо-режим" });
    expect(banner).toHaveTextContent("Демо-режим: данные синтетические, изменения не сохраняются.");
    expect(within(banner).getByRole("link", { name: "Выйти из демо" })).toHaveAttribute(
      "href",
      "/demo/exit",
    );
  });

  it("speaks English", () => {
    renderWithLocale(<DemoBanner locale="en" />, "en");

    const banner = screen.getByRole("region", { name: "Demo mode" });
    expect(banner).toHaveTextContent("Demo mode: the data is synthetic and changes aren't saved.");
    expect(within(banner).getByRole("link", { name: "Exit demo" })).toHaveAttribute(
      "href",
      "/demo/exit",
    );
  });
});
