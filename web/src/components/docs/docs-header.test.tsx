import { screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { getGetAuthStateMockHandler } from "@/lib/api/gen/endpoints/admin-auth/admin-auth.msw";
import { getListAppsMockHandler } from "@/lib/api/gen/endpoints/apps/apps.msw";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { DocsHeader } from "./docs-header";

vi.mock("next/navigation", () => ({
  usePathname: () => "/docs",
  useSearchParams: () => new URLSearchParams(),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
}));

// Гость не должен получать шапку дашборда: она грузит список приложений, а 401 увёл бы его на вход.
describe("DocsHeader", () => {
  it("shows a guest the public header with sign-in", async () => {
    server.use(getGetAuthStateMockHandler({ authenticated: false, setup_required: false }));

    renderWithClient(await DocsHeader());

    const header = screen.getByRole("banner");
    expect(within(header).getByRole("link", { name: "Войти" })).toHaveAttribute("href", "/login");
    expect(within(header).queryByRole("link", { name: "Портфель" })).not.toBeInTheDocument();
  });

  // Телефон: шапка уезжает при прокрутке (сверху остаётся полоса Fumadocs с поиском и меню), а её слой
  // ниже выдвижного сайдбара (z-40) и поиска (z-50) Fumadocs — они не прячутся под шапкой.
  it("scrolls away on phones and stays under the Fumadocs drawer and search", async () => {
    server.use(getGetAuthStateMockHandler({ authenticated: false, setup_required: false }));

    renderWithClient(await DocsHeader());

    const bar = screen.getByRole("banner").parentElement;
    expect(bar).toHaveClass("md:sticky", "md:top-0", "z-35");
    expect(bar).not.toHaveClass("sticky");
  });

  it("wraps the owner's dashboard header the same way", async () => {
    server.use(
      getGetAuthStateMockHandler({ authenticated: true, setup_required: false }),
      getListAppsMockHandler({ items: [] }),
    );

    renderWithClient(await DocsHeader());

    expect(screen.getByRole("banner").parentElement).toHaveClass("md:sticky", "z-35");
  });

  it("shows the owner the dashboard header", async () => {
    server.use(
      getGetAuthStateMockHandler({ authenticated: true, setup_required: false }),
      getListAppsMockHandler({ items: [] }),
    );

    renderWithClient(await DocsHeader());

    const nav = screen.getByRole("navigation", { name: "Основная навигация" });
    expect(within(nav).getByRole("link", { name: "Портфель" })).toHaveAttribute(
      "href",
      "/portfolio",
    );
  });
});
