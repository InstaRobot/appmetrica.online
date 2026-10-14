import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Platform } from "@/lib/api/gen/model";
import { PLATFORM_LABELS, PlatformIcon } from "./platform-icon";

describe("PlatformIcon", () => {
  it.each(Object.values(Platform))("renders labelled icon for %s", (platform) => {
    render(<PlatformIcon platform={platform} title={PLATFORM_LABELS[platform]} />);

    const icon = screen.getByRole("img", { name: PLATFORM_LABELS[platform] });
    expect(icon.tagName.toLowerCase()).toBe("svg");
    expect(icon).toHaveAttribute("data-platform-icon");
  });

  it("uses the same glyph for apple and desktop families", () => {
    const glyph = (platform: string) => {
      const { container, unmount } = render(<PlatformIcon platform={platform} />);
      const name = container.querySelector("svg")?.getAttribute("data-platform-icon");
      unmount();
      return name;
    };

    expect(glyph("ios")).toBe("apple");
    expect(glyph("macos")).toBe("apple");
    expect(glyph("windows")).toBe("desktop");
    expect(glyph("linux")).toBe("desktop");
    expect(glyph("tvos-unknown")).toBe("other");
  });

  it("is decorative without title", () => {
    const { container } = render(<PlatformIcon platform="android" className="size-5" />);

    const svg = container.querySelector("svg");
    expect(svg).toHaveAttribute("aria-hidden", "true");
    expect(svg).toHaveClass("size-5");
    expect(screen.queryByRole("img")).toBeNull();
  });
});
