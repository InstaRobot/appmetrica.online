import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import DocsNotFound from "./[lang]/docs/not-found";
import NotFound from "./not-found";

describe("S-14a not-found", () => {
  it("shows the heading and a link back to the portfolio", () => {
    render(<NotFound />);

    expect(screen.getByRole("heading", { level: 1, name: "Страница не найдена" })).toBeVisible();
    expect(screen.getByText("Проверьте адрес или вернитесь в портфель.")).toBeVisible();
    expect(screen.getByRole("link", { name: "В портфель" })).toHaveAttribute("href", "/portfolio");
  });

  // Страница статическая и стоит вне словаря: в разметке оба языка, CSS показывает тот, что inline-скрипт
  // записал в <html lang> до первой отрисовки.
  it("carries English too and shows the language of <html lang>", () => {
    render(<NotFound />);

    const en = screen.getByRole("heading", { level: 1, name: "Page not found" }).parentElement;
    const ru = screen.getByRole("heading", { level: 1, name: "Страница не найдена" }).parentElement;
    expect(en).toHaveAttribute("lang", "en");
    expect(en).toHaveClass("[html[lang=ru]_&]:hidden");
    expect(ru).toHaveAttribute("lang", "ru");
    expect(ru).toHaveClass("[html:not([lang=ru])_&]:hidden");
    expect(en).toHaveTextContent("Check the address or go back to the portfolio.");
    expect(screen.getByRole("link", { name: "Go to portfolio" })).toHaveAttribute(
      "href",
      "/portfolio",
    );
  });
});

// 404 документации — внутри layout [lang], словарь и язык берутся из провайдера.
describe("S-13 docs not-found", () => {
  it("speaks English and leads to the English quick start", () => {
    renderWithLocale(<DocsNotFound />, "en");

    expect(screen.getByRole("heading", { level: 1, name: "Page not found" })).toBeVisible();
    expect(screen.getByText("There's no such page in the documentation.")).toBeVisible();
    expect(screen.getByRole("link", { name: "Go to quick start" })).toHaveAttribute(
      "href",
      "/docs",
    );
  });

  it("speaks Russian and leads to the Russian quick start", () => {
    render(<DocsNotFound />);

    expect(screen.getByRole("heading", { level: 1, name: "Страница не найдена" })).toBeVisible();
    expect(screen.getByText("Такой страницы в документации нет.")).toBeVisible();
    expect(screen.getByRole("link", { name: "К быстрому старту" })).toHaveAttribute(
      "href",
      "/ru/docs",
    );
  });
});
