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

  it("renders without optional parts", () => {
    const { container } = render(<PageHeader title="Портфель" />);

    expect(screen.getByRole("heading", { level: 1, name: "Портфель" })).toBeVisible();
    expect(container.querySelector("p")).toBeNull();
  });
});
