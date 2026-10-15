import { render, renderHook, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { Platform } from "@/lib/api/gen/model";
import { MessagesProvider } from "@/lib/i18n/client";
import { getMessages } from "@/messages";
import { PlatformIcon, platformLabelIn, usePlatformLabel } from "./platform-icon";

const RU_LABELS = getMessages("ru").common.platforms;

describe("PlatformIcon", () => {
  it.each(Object.values(Platform))("renders labelled icon for %s", (platform) => {
    render(<PlatformIcon platform={platform} title={RU_LABELS[platform]} />);

    const icon = screen.getByRole("img", { name: RU_LABELS[platform] });
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

describe("platformLabelIn", () => {
  it("labels known, unknown and empty platforms with the Russian dictionary", () => {
    expect(platformLabelIn(RU_LABELS, "ios")).toBe("iOS");
    expect(platformLabelIn(RU_LABELS, "server")).toBe("Бэкенд");
    expect(platformLabelIn(RU_LABELS, "tvos")).toBe("tvos");
    expect(platformLabelIn(RU_LABELS, "")).toBe("—");
  });
});

describe("platform labels of the interface language", () => {
  it("labels known, unknown and empty platforms from the dictionary", () => {
    const labels = getMessages("en").common.platforms;

    expect(platformLabelIn(labels, "web")).toBe("Web");
    expect(platformLabelIn(labels, "server")).toBe("Backend");
    expect(platformLabelIn(labels, "tvos")).toBe("tvos");
    expect(platformLabelIn(labels, "")).toBe("—");
  });

  it("takes the labels of the current locale", () => {
    const english = ({ children }: { children: ReactNode }) => (
      <MessagesProvider locale="en" messages={getMessages("en")}>
        {children}
      </MessagesProvider>
    );

    expect(renderHook(() => usePlatformLabel()).result.current("bot")).toBe("Бот");
    expect(renderHook(() => usePlatformLabel(), { wrapper: english }).result.current("bot")).toBe(
      "Bot",
    );
  });
});
