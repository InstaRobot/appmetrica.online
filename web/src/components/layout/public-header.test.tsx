import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PublicHeader } from "./public-header";

// Шапка публичных страниц (лендинг, документация для гостя): без запросов к API.
describe("PublicHeader", () => {
  it("links the logo home and offers the docs, theme switch and sign-in", () => {
    render(<PublicHeader />);

    const header = screen.getByRole("banner");
    expect(
      within(header).getByRole("link", { name: "appmetrica.online — на главную" }),
    ).toHaveAttribute("href", "/");
    expect(within(header).getByRole("link", { name: "Документация" })).toHaveAttribute(
      "href",
      "/docs",
    );
    expect(within(header).getByRole("link", { name: "Войти" })).toHaveAttribute("href", "/login");
    expect(within(header).getByRole("button", { name: /тему$/ })).toBeInTheDocument();
  });
});
