import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LayoutGrid } from "lucide-react";
import { describe, expect, it, vi } from "vitest";
import { EmptyState } from "./empty-state";

describe("EmptyState", () => {
  it("renders icon, title, description and link action", () => {
    const { container } = render(
      <EmptyState
        icon={LayoutGrid}
        title="Пока нет приложений"
        description="Добавьте первое приложение."
        action={{ label: "Добавить приложение", href: "/apps/new" }}
      />,
    );

    expect(screen.getByRole("heading", { name: "Пока нет приложений" })).toBeVisible();
    expect(screen.getByText("Добавьте первое приложение.")).toBeVisible();
    expect(screen.getByRole("link", { name: "Добавить приложение" })).toHaveAttribute(
      "href",
      "/apps/new",
    );
    expect(container.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  });

  it("renders button action and nothing extra without action", async () => {
    const onClick = vi.fn();
    const { rerender } = render(
      <EmptyState icon={LayoutGrid} title="Пусто" action={{ label: "Сбросить", onClick }} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Сбросить" }));
    expect(onClick).toHaveBeenCalledOnce();

    rerender(<EmptyState icon={LayoutGrid} title="Пусто" />);
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByRole("link")).toBeNull();
  });
});
