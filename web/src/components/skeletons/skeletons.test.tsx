import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { IntegrationSkeleton } from "./integration-skeleton";
import { KpiSkeleton } from "./kpi-skeleton";
import { ProfileSkeleton } from "./profile-skeleton";
import { RealtimeSkeleton } from "./realtime-skeleton";
import { RetentionSkeleton } from "./retention-skeleton";
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

  it("renders a 10 × 12 retention grid with the chart placeholder", () => {
    const { container } = render(<RetentionSkeleton />);

    expect(screen.getAllByRole("status", { name: "Загрузка…" })).not.toHaveLength(0);
    expect(container.querySelectorAll("[data-skeleton-cell]")).toHaveLength(120);
  });

  it("renders the realtime counter, chart and eight feed rows", () => {
    const { container } = render(<RealtimeSkeleton />);

    expect(screen.getByRole("status", { name: "Загрузка…" })).toBeInTheDocument();
    expect(container.querySelector("[data-skeleton-counter]")).toHaveClass("h-14");
    expect(container.querySelectorAll("[data-skeleton-row]")).toHaveLength(8);
  });

  it("renders the integration key card and snippet placeholder", () => {
    const { container } = render(<IntegrationSkeleton />);

    expect(screen.getByRole("status", { name: "Загрузка…" })).toBeInTheDocument();
    expect(container.querySelector("[data-skeleton-code]")).toHaveClass("h-80");
  });

  it("renders the profile card and two tables", () => {
    const { container } = render(<ProfileSkeleton />);

    expect(screen.getAllByRole("status", { name: "Загрузка…" })).toHaveLength(1);
    expect(container.querySelectorAll("[data-skeleton-card]")).toHaveLength(1);
    expect(container.querySelectorAll("[data-skeleton-row]")).toHaveLength(7);
  });
});
