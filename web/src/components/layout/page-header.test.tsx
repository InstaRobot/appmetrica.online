import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PageHeader } from "./page-header";

describe("PageHeader", () => {
  it("renders single h1 with adornment, description and actions", () => {
    render(
      <PageHeader
        title="Notes iOS"
        adornment={<span>iOS</span>}
        description="Аудитория приложения"
        actions={<button type="button">Добавить приложение</button>}
      />,
    );

    const headings = screen.getAllByRole("heading");
    expect(headings).toHaveLength(1);
    expect(headings[0]).toHaveTextContent("Notes iOS");
    expect(headings[0]?.tagName).toBe("H1");
    expect(screen.getByText("iOS")).toBeVisible();
    expect(screen.getByText("Аудитория приложения")).toBeVisible();
    expect(screen.getByRole("button", { name: "Добавить приложение" })).toBeVisible();
  });

  // jsdom не считает CSS: перенос на телефоне проверяется по брейкпоинт-классам.
  it("wraps badges under the title and actions below it on a phone", () => {
    render(
      <PageHeader
        title="Notes iOS"
        adornment={<span>iOS</span>}
        actions={<button type="button">Добавить приложение</button>}
      />,
    );

    const heading = screen.getByRole("heading", { level: 1 });
    expect(heading).toHaveClass("truncate");
    expect(heading.parentElement).toHaveClass("flex-wrap", "md:flex-nowrap");
    expect(screen.getByRole("banner")).toHaveClass("flex-col", "md:flex-row");
    expect(screen.getByRole("button", { name: "Добавить приложение" }).parentElement).toHaveClass(
      "flex-wrap",
      "md:shrink-0",
    );
  });

  it("renders without optional parts", () => {
    const { container } = render(<PageHeader title="Портфель" />);

    expect(screen.getByRole("heading", { level: 1, name: "Портфель" })).toBeVisible();
    expect(container.querySelector("p")).toBeNull();
  });
});
