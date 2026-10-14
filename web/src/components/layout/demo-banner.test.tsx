import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DemoBanner } from "./demo-banner";

describe("DemoBanner", () => {
  it("explains the synthetic data and offers a way out", () => {
    render(<DemoBanner />);

    const banner = screen.getByRole("region", { name: "Демо-режим" });
    expect(banner).toHaveTextContent("Демо-режим: данные синтетические, изменения не сохраняются.");
    expect(within(banner).getByRole("link", { name: "Выйти из демо" })).toHaveAttribute(
      "href",
      "/demo/exit",
    );
  });
});
