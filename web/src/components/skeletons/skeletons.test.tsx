import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { KpiSkeleton } from "./kpi-skeleton";
import { TableSkeleton } from "./table-skeleton";

describe("skeletons", () => {
  it("renders requested number of table rows and announces loading", () => {
    const { container } = render(<TableSkeleton rows={4} />);

    expect(screen.getByRole("status", { name: "Загрузка…" })).toBeInTheDocument();
    expect(container.querySelectorAll("[data-skeleton-row]")).toHaveLength(4);
  });

  it("renders KPI grid placeholders", () => {
    const { container } = render(<KpiSkeleton count={4} />);

    expect(screen.getByRole("status", { name: "Загрузка…" })).toBeInTheDocument();
    expect(container.querySelectorAll("[data-skeleton-kpi]")).toHaveLength(4);
  });
});
