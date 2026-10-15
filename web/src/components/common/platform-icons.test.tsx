import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { PlatformIcons } from "./platform-icons";

const glyphs = (container: HTMLElement) =>
  Array.from(container.querySelectorAll("svg")).map((svg) =>
    svg.getAttribute("data-platform-icon"),
  );

describe("PlatformIcons", () => {
  it("collapses extra platforms", () => {
    const { container } = render(
      <PlatformIcons platforms={["ios", "android", "web", "macos"]} max={3} />,
    );

    expect(glyphs(container)).toEqual(["apple", "android", "web"]);
    expect(screen.getByText("+1")).toBeInTheDocument();
    expect(screen.getByTitle("iOS, Android, Веб, macOS")).toBeInTheDocument();
  });

  it("names the platforms in English", () => {
    renderWithLocale(<PlatformIcons platforms={["web", "server", "tvos"]} />, "en");

    expect(screen.getByTitle("Web, Backend, tvos")).toBeInTheDocument();
  });

  it("shows every platform within the limit", () => {
    const { container } = render(<PlatformIcons platforms={["android", "web"]} />);

    expect(glyphs(container)).toEqual(["android", "web"]);
    expect(screen.queryByText(/^\+/)).not.toBeInTheDocument();
    expect(screen.getByTitle("Android, Веб")).toBeInTheDocument();
  });

  it("draws an unknown value as other and names it as is", () => {
    const { container } = render(<PlatformIcons platforms={["tvos"]} />);

    expect(glyphs(container)).toEqual(["other"]);
    expect(screen.getByTitle("tvos")).toBeInTheDocument();
  });

  it("stays out of the accessible name of a link", () => {
    render(
      <a href="/apps/1">
        <PlatformIcons platforms={["ios", "android", "web", "macos"]} max={2} />
        Notes
      </a>,
    );

    expect(screen.getByRole("link")).toHaveAccessibleName("Notes");
  });

  it("renders nothing without platforms", () => {
    const { container } = render(<PlatformIcons platforms={[]} />);

    expect(container).toBeEmptyDOMElement();
  });
});
