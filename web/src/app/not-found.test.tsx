import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import NotFound from "./not-found";

describe("S-14a not-found", () => {
  it("shows the heading and a link back to the portfolio", () => {
    render(<NotFound />);

    expect(screen.getByRole("heading", { level: 1, name: "Страница не найдена" })).toBeVisible();
    expect(screen.getByText("Проверьте адрес или вернитесь в портфель.")).toBeVisible();
    expect(screen.getByRole("link", { name: "В портфель" })).toHaveAttribute("href", "/portfolio");
  });
});
