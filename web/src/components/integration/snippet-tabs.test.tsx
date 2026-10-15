import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { SnippetTabs } from "./snippet-tabs";

const KEY = "pk_7Hq2x9AbLm3Np4Qr5St6Uv7Wx8Yz9Ab0";
const URL = "http://localhost:8081/v1/events";

describe("SnippetTabs", () => {
  it("opens the platform language and switches tabs with arrows", async () => {
    const user = userEvent.setup();
    render(<SnippetTabs ingestUrl={URL} writeKey={KEY} platform="ios" />);

    const tabs = within(screen.getByRole("tablist", { name: "Язык сниппета" })).getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(["curl", "Swift", "Kotlin", "JavaScript", "Go"]);
    expect(screen.getByRole("tab", { name: "Swift" })).toHaveAttribute("aria-selected", "true");

    const swift = screen.getByLabelText("Пример кода на Swift");
    expect(swift.textContent).toContain(`static let writeKey = "${KEY}"`);
    expect(swift.textContent).toContain(`URL(string: "${URL}")!`);
    expect(screen.getByRole("link", { name: /Полные рецепты/ })).toHaveAttribute(
      "href",
      "/ru/docs/recipes/ios-swift",
    );

    await user.click(screen.getByRole("tab", { name: "Swift" }));
    await user.keyboard("{ArrowRight}");
    const kotlin = screen.getByRole("tab", { name: "Kotlin" });
    expect(kotlin).toHaveFocus();
    expect(kotlin).toHaveAttribute("aria-selected", "true");
    expect(screen.getByLabelText("Пример кода на Kotlin").textContent).toContain(
      `private const val WRITE_KEY = "${KEY}"`,
    );
    expect(screen.getByRole("link", { name: /Полные рецепты/ })).toHaveAttribute(
      "href",
      "/ru/docs/recipes/android-kotlin",
    );
  });

  it("uses the default language and falls back to curl", () => {
    const { unmount } = render(
      <SnippetTabs ingestUrl={URL} writeKey={KEY} platform="ios" defaultLang="go" />,
    );
    expect(screen.getByRole("tab", { name: "Go" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("link", { name: /Полные рецепты/ })).toHaveAttribute(
      "href",
      "/ru/docs/recipes/go",
    );
    unmount();

    render(<SnippetTabs ingestUrl={URL} writeKey={KEY} platform="windows" />);
    expect(screen.getByRole("tab", { name: "curl" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByLabelText("Пример кода на curl").textContent).toContain(
      `-H "X-App-Key: ${KEY}"`,
    );
    expect(screen.getByRole("link", { name: /Полные рецепты/ })).toHaveAttribute(
      "href",
      "/ru/docs/recipes/curl",
    );
  });

  it("labels tabs and code in English and links to the English docs", () => {
    renderWithLocale(<SnippetTabs ingestUrl={URL} writeKey={KEY} platform="ios" />, "en");

    expect(screen.getByRole("tablist", { name: "Snippet language" })).toBeInTheDocument();
    const swift = screen.getByLabelText("Code example in Swift");
    expect(swift.textContent).toContain(
      "// Start a session when the app comes to the foreground. InstallID.current is a UUID from the Keychain",
    );
    expect(
      screen.getByRole("link", {
        name: "Full recipes with an offline queue and sessions are in the docs →",
      }),
    ).toHaveAttribute("href", "/docs/recipes/ios-swift");
  });
});
