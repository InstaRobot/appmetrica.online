import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { docsHref, docsLink } from "./docs-link";

describe("docsHref", () => {
  it("moves internal docs links under /ru on Russian pages", () => {
    expect(docsHref("/docs/recipes/curl#batch", "ru")).toBe("/ru/docs/recipes/curl#batch");
    expect(docsHref("/docs", "ru")).toBe("/ru/docs");
    expect(docsHref("/docs/recipes/curl", "en")).toBe("/docs/recipes/curl");
  });

  it("keeps anchors, other pages and external links", () => {
    expect(docsHref("#batch", "ru")).toBe("#batch");
    expect(docsHref("/login", "ru")).toBe("/login");
    expect(docsHref("https://example.com/docs/x", "ru")).toBe("https://example.com/docs/x");
    expect(docsHref("/docsx", "ru")).toBe("/docsx");
    expect(docsHref(undefined, "ru")).toBeUndefined();
  });
});

describe("docsLink", () => {
  it("renders MDX links with the localized address", () => {
    const A = docsLink("ru");
    render(<A href="/docs/concepts/sessions">Сессии</A>);
    expect(screen.getByRole("link", { name: "Сессии" })).toHaveAttribute(
      "href",
      "/ru/docs/concepts/sessions",
    );
  });
});
