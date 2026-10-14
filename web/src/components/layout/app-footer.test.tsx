import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AppFooter } from "./app-footer";
import { SkipLink } from "./skip-link";

describe("AppFooter", () => {
  it("shows DB-IP attribution", () => {
    render(<AppFooter />);

    expect(screen.getByRole("contentinfo")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "IP Geolocation by DB-IP" })).toHaveAttribute(
      "href",
      "https://db-ip.com",
    );
    expect(screen.getByRole("link", { name: "Документация" })).toHaveAttribute("href", "/docs");
  });
});

describe("SkipLink", () => {
  it("points to main content", () => {
    render(<SkipLink />);

    expect(screen.getByRole("link", { name: "Перейти к содержимому" })).toHaveAttribute(
      "href",
      "#main",
    );
  });
});
