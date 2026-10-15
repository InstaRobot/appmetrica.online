import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { PublicHeader } from "./public-header";

// Шапка публичных страниц (лендинг, документация для гостя): без запросов к API.
describe("PublicHeader", () => {
  it("links the logo home and offers the docs, theme and language switches and sign-in", () => {
    render(<PublicHeader />);

    const header = screen.getByRole("banner");
    expect(
      within(header).getByRole("link", { name: "appmetrica.online — на главную" }),
    ).toHaveAttribute("href", "/ru");
    expect(within(header).getByRole("link", { name: "Документация" })).toHaveAttribute(
      "href",
      "/ru/docs",
    );
    expect(within(header).getByRole("link", { name: "Войти" })).toHaveAttribute("href", "/login");
    expect(within(header).getByRole("button", { name: /тему$/ })).toBeInTheDocument();
    expect(
      within(header).getByRole("button", { name: "Переключить на English" }),
    ).toBeInTheDocument();
  });

  it("speaks English and links the English pages", () => {
    renderWithLocale(<PublicHeader />, "en");

    const header = screen.getByRole("banner");
    expect(within(header).getByRole("link", { name: "appmetrica.online — home" })).toHaveAttribute(
      "href",
      "/",
    );
    expect(within(header).getByRole("link", { name: "Documentation" })).toHaveAttribute(
      "href",
      "/docs",
    );
    expect(within(header).getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login");
    expect(within(header).getByRole("button", { name: "Switch to Русский" })).toHaveTextContent(
      "RU",
    );
  });
});
