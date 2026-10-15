import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { portfolioRow } from "@/test/factories";
import { renderWithClient } from "@/test/render";
import { renderWithLocale } from "@/test/rtl";
import { PlatformShareBar } from "./platform-share-bar";
import { PortfolioTable } from "./portfolio-table";

// В доступном имени U+00A0 сохраняется, а toHaveTextContent сводит его к обычному пробелу.
const NBSP = " ";

const notes = portfolioRow({
  app: {
    id: "0192aaaa-0000-7000-8000-000000000001",
    name: "Notes",
    platform: "ios",
    platforms: ["ios", "android", "web"],
    status: "active",
  },
  active_date: 9500,
  platforms: [
    { platform: "ios", active: 5210 },
    { platform: "android", active: 3870 },
    { platform: "web", active: 1120 },
  ],
});

describe("PlatformShareBar", () => {
  it("labels the shares", () => {
    renderWithClient(
      <PortfolioTable
        rows={[notes]}
        date="2026-10-12"
        sort={{ key: "active_date", desc: true }}
        onSortChange={vi.fn()}
      />,
    );

    const row = screen.getAllByRole("row")[1];
    if (!row) throw new Error("no body row");
    expect(
      within(row).getByRole("img", {
        name: `iOS 51${NBSP}%, Android 38${NBSP}%, Веб 11${NBSP}%`,
      }),
    ).toBeInTheDocument();
  });

  it("explains absolutes and the overlap in a focusable tooltip", async () => {
    const user = userEvent.setup();
    render(<PlatformShareBar items={notes.platforms} total={9500} />);

    await user.tab();
    expect(screen.getByRole("img")).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "iOS 5 210 · Android 3 870 · Веб 1 120. Человек на нескольких платформах учтён в каждой",
    );
  });

  it("draws a segment per platform in its chart color", () => {
    const { container } = render(<PlatformShareBar items={notes.platforms} total={9500} />);

    const segments = Array.from(container.querySelectorAll<HTMLElement>("[data-platform]"));
    expect(segments.map((s) => s.dataset.platform)).toEqual(["ios", "android", "web"]);
    expect(segments[0]?.style.backgroundColor).toBe("var(--chart-1)");
    expect(segments[1]?.style.backgroundColor).toBe("var(--chart-3)");
    expect(segments[0]?.style.width).toMatch(/^51\.07/);
  });

  it("omits the overlap note when nobody is counted twice", async () => {
    const user = userEvent.setup();
    render(<PlatformShareBar items={[{ platform: "web", active: 3117 }]} total={3117} />);

    expect(screen.getByRole("img", { name: `Веб 100${NBSP}%` })).toBeInTheDocument();
    await user.tab();
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("Веб 3 117");
    expect(tooltip).not.toHaveTextContent("Человек на нескольких платформах");
  });

  it("names an unknown platform as is", () => {
    render(
      <PlatformShareBar
        items={[
          { platform: "ios", active: 30 },
          { platform: "tvos", active: 10 },
        ]}
        total={40}
      />,
    );

    expect(
      screen.getByRole("img", { name: `iOS 75${NBSP}%, tvos 25${NBSP}%` }),
    ).toBeInTheDocument();
  });

  it("shows a dash without active people", () => {
    const { rerender } = render(<PlatformShareBar items={[]} total={0} />);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByText("—")).toBeInTheDocument();

    rerender(<PlatformShareBar items={[{ platform: "ios", active: 0 }]} total={0} />);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("labels the shares and explains the overlap in English", async () => {
    const user = userEvent.setup();
    const items = notes.platforms.filter((p) => p.platform !== "web");
    renderWithLocale(<PlatformShareBar items={items} total={8000} />, "en");

    expect(screen.getByRole("img", { name: "iOS 57%, Android 43%" })).toBeInTheDocument();
    await user.tab();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "iOS 5,210 · Android 3,870. A person on several platforms is counted in each",
    );
  });
});
