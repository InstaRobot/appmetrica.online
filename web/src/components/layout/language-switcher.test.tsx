import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, renderWithLocale } from "@/test/rtl";
import { LanguageSwitcher } from "./language-switcher";

const nav = vi.hoisted(() => ({ push: vi.fn(), refresh: vi.fn() }));

vi.mock("@/lib/i18n/navigate", () => ({ openPage: nav.push, reloadPage: nav.refresh }));

describe("LanguageSwitcher", () => {
  beforeEach(() => {
    nav.push.mockClear();
    nav.refresh.mockClear();
    // biome-ignore lint/suspicious/noDocumentCookie: сброс выбора языка между тестами
    document.cookie = "am_lang=; max-age=0; path=/";
  });

  it("opens the paired page of a public address and saves the choice", async () => {
    window.history.pushState({}, "", "/docs/recipes/curl?tab=1#queue");
    renderWithLocale(<LanguageSwitcher />, "en");

    const button = screen.getByRole("button", { name: "Switch to Русский" });
    expect(button).toHaveTextContent("RU");
    expect(button).toHaveAttribute("lang", "ru");
    await userEvent.click(button);

    expect(document.cookie).toContain("am_lang=ru");
    expect(nav.push).toHaveBeenCalledWith("/ru/docs/recipes/curl?tab=1#queue");
    expect(nav.refresh).not.toHaveBeenCalled();
  });

  it("refreshes the panel in place: its address has no language", async () => {
    window.history.pushState({}, "", "/portfolio");
    render(<LanguageSwitcher />);

    await userEvent.click(screen.getByRole("button", { name: "Переключить на English" }));

    expect(document.cookie).toContain("am_lang=en");
    expect(nav.refresh).toHaveBeenCalledOnce();
    expect(nav.push).not.toHaveBeenCalled();
  });
});
