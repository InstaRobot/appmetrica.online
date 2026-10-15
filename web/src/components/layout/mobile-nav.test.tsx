import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderWithClient } from "@/test/render";
import { MobileNav } from "./mobile-nav";

vi.mock("next/navigation", () => ({
  usePathname: () => "/portfolio",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
}));

const panelItems = [
  { href: "/portfolio" as const, label: "Портфель", active: true },
  { href: "/ru/docs" as const, label: "Документация" },
  { href: "/settings" as const, label: "Настройки" },
];

// Навигация на телефоне (< md): кнопка ☰ открывает боковую панель с разделами, языком, темой и выходом.
describe("MobileNav", () => {
  it("opens a panel with sections, language, theme and sign-out", async () => {
    const user = userEvent.setup();
    renderWithClient(<MobileNav variant="panel" items={panelItems} />);

    const button = screen.getByRole("button", { name: "Открыть меню" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    await user.click(button);

    const panel = await screen.findByRole("dialog", { name: "Навигация" });
    expect(within(panel).getByRole("link", { name: "Портфель" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(panel).getByRole("link", { name: "Документация" })).toHaveAttribute(
      "href",
      "/ru/docs",
    );
    expect(within(panel).getByRole("link", { name: "Настройки" })).toHaveAttribute(
      "href",
      "/settings",
    );
    expect(
      within(panel).getByRole("button", { name: "Переключить на English" }),
    ).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: /тему$/ })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Выйти" })).toBeInTheDocument();
  });

  it("closes the panel when a section is chosen", async () => {
    const user = userEvent.setup();
    renderWithClient(<MobileNav variant="panel" items={panelItems} />);

    await user.click(screen.getByRole("button", { name: "Открыть меню" }));
    const panel = await screen.findByRole("dialog", { name: "Навигация" });
    await user.click(within(panel).getByRole("link", { name: "Настройки" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("offers to leave the demo instead of signing out", async () => {
    const user = userEvent.setup();
    renderWithClient(<MobileNav variant="panel" demo items={panelItems} />);

    await user.click(screen.getByRole("button", { name: "Открыть меню" }));
    const panel = await screen.findByRole("dialog", { name: "Навигация" });
    expect(within(panel).getByRole("link", { name: "Выйти из демо" })).toHaveAttribute(
      "href",
      "/demo/exit",
    );
    expect(within(panel).queryByRole("button", { name: "Выйти" })).not.toBeInTheDocument();
  });

  it("has no sign-out on public pages and speaks English", async () => {
    const user = userEvent.setup();
    renderWithClient(
      <MobileNav variant="public" items={[{ href: "/docs" as const, label: "Documentation" }]} />,
      { locale: "en" },
    );

    await user.click(screen.getByRole("button", { name: "Open menu" }));
    const panel = await screen.findByRole("dialog", { name: "Navigation" });
    expect(within(panel).getByRole("link", { name: "Documentation" })).toHaveAttribute(
      "href",
      "/docs",
    );
    expect(within(panel).getByRole("button", { name: "Switch to Русский" })).toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: "Sign out" })).not.toBeInTheDocument();
  });
});
