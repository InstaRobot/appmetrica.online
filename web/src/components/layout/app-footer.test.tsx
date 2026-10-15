import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { AppFooter } from "./app-footer";
import { SkipLink } from "./skip-link";

describe("AppFooter", () => {
  it("shows DB-IP attribution", () => {
    render(<AppFooter locale="ru" />);

    expect(screen.getByRole("contentinfo")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "IP Geolocation by DB-IP" })).toHaveAttribute(
      "href",
      "https://db-ip.com",
    );
    expect(screen.getByRole("link", { name: "Документация" })).toHaveAttribute("href", "/ru/docs");
  });

  it("speaks English and links the English docs", () => {
    renderWithLocale(<AppFooter locale="en" />, "en");

    expect(screen.getByRole("link", { name: "Documentation" })).toHaveAttribute("href", "/docs");
    expect(screen.getByRole("link", { name: "IP Geolocation by DB-IP" })).toBeVisible();
  });
});

describe("SkipLink", () => {
  it("points to main content", () => {
    render(<SkipLink locale="ru" />);

    expect(screen.getByRole("link", { name: "Перейти к содержимому" })).toHaveAttribute(
      "href",
      "#main",
    );
  });

  it("speaks English", () => {
    renderWithLocale(<SkipLink locale="en" />, "en");

    expect(screen.getByRole("link", { name: "Skip to content" })).toHaveAttribute("href", "#main");
  });
});
