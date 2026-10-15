import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getListAppsMockHandler } from "@/lib/api/gen/endpoints/apps/apps.msw";
import { MessagesProvider } from "@/lib/i18n/client";
import { getMessages } from "@/messages";
import { errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { AppHeader } from "./app-header";
import { AppTabs } from "./app-tabs";

const nav = vi.hoisted(() => ({
  pathname: "/portfolio",
  search: "",
  push: vi.fn<(href: string) => void>(),
  replace: vi.fn<(href: string) => void>(),
}));
vi.mock("next/navigation", () => ({
  usePathname: () => nav.pathname,
  useSearchParams: () => new URLSearchParams(nav.search),
  useRouter: () => ({ push: nav.push, replace: nav.replace, refresh: vi.fn(), prefetch: vi.fn() }),
}));

const notes = makeApp({ id: "0192aaaa-0000-7000-8000-000000000001", name: "Notes iOS" });
const maps = makeApp({
  id: "0192aaaa-0000-7000-8000-000000000002",
  name: "Maps Android",
  platform: "android",
});

function at(pathname: string, search = "") {
  nav.pathname = pathname;
  nav.search = search;
}

const mainNav = () => screen.getByRole("navigation", { name: "Основная навигация" });
const appNav = () => screen.getByRole("navigation", { name: "Разделы приложения" });
const switcher = () => screen.findByRole("combobox", { name: "Приложение" });
// Список приложений грузится на клиенте: ждём, пока триггер покажет ожидаемое имя.
const expectSwitcher = (text: string) =>
  waitFor(() =>
    expect(screen.getByRole("combobox", { name: "Приложение" })).toHaveTextContent(text),
  );

beforeEach(() => {
  at("/portfolio");
  nav.push.mockReset();
  nav.replace.mockReset();
  server.use(getListAppsMockHandler({ items: [notes, maps] }));
});

describe("AppHeader", () => {
  it("marks active tab and current app", async () => {
    at(`/apps/${notes.id}/retention`, "period=30d&platform=ios&metric=new");

    renderWithClient(
      <>
        <AppHeader />
        <AppTabs appId={notes.id} />
      </>,
    );

    const tabs = within(appNav()).getAllByRole("link");
    expect(tabs.map((t) => t.textContent)).toEqual([
      "Аудитория",
      "Retention",
      "Realtime",
      "Профили",
      "Интеграция",
      "Настройки",
    ]);
    const retention = within(appNav()).getByRole("link", { name: "Retention" });
    expect(retention).toHaveAttribute("aria-current", "page");
    for (const tab of tabs.filter((t) => t !== retention)) {
      expect(tab).not.toHaveAttribute("aria-current");
    }
    await expectSwitcher("Notes iOS");
    // Внутри приложения пункт «Портфель» не активен.
    expect(within(mainNav()).getByRole("link", { name: "Портфель" })).not.toHaveAttribute(
      "aria-current",
    );
  });

  it("keeps report query params on tab links", () => {
    at(`/apps/${notes.id}`, "period=30d&platform=ios&platform=android&env=development&metric=new");

    renderWithClient(<AppTabs appId={notes.id} />);

    expect(within(appNav()).getByRole("link", { name: "Аудитория" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(appNav()).getByRole("link", { name: "Realtime" })).toHaveAttribute(
      "href",
      `/apps/${notes.id}/realtime?period=30d&platform=ios&platform=android&env=development`,
    );
    expect(within(appNav()).getByRole("link", { name: "Настройки" })).toHaveAttribute(
      "href",
      `/apps/${notes.id}/settings?period=30d&platform=ios&platform=android&env=development`,
    );
  });

  it("marks nested pages of a tab as active", () => {
    at(`/apps/${notes.id}/profiles/u%3A42`);

    renderWithClient(<AppTabs appId={notes.id} />);

    expect(within(appNav()).getByRole("link", { name: "Профили" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(appNav()).getByRole("link", { name: "Аудитория" })).toHaveAttribute(
      "href",
      `/apps/${notes.id}`,
    );
  });

  it("shows all apps on portfolio and marks portfolio link", async () => {
    renderWithClient(<AppHeader />);

    expect(within(mainNav()).getByRole("link", { name: "Портфель" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(mainNav()).getByRole("link", { name: "Документация" })).toHaveAttribute(
      "href",
      "/ru/docs",
    );
    await expectSwitcher("Все приложения");
    expect(screen.getByRole("link", { name: "appmetrica.online — в портфель" })).toHaveAttribute(
      "href",
      "/portfolio",
    );
  });

  it("marks settings link on settings page and does not treat /apps/new as an app", async () => {
    at("/settings");
    const { unmount } = renderWithClient(<AppHeader />);
    expect(within(mainNav()).getByRole("link", { name: "Настройки" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    unmount();

    let loaded = false;
    server.use(
      getListAppsMockHandler(() => {
        loaded = true;
        return { items: [notes, maps] };
      }),
    );
    at("/apps/new");
    renderWithClient(<AppHeader />);
    await waitFor(() => expect(loaded).toBe(true));
    await expectSwitcher("Все приложения");
  });

  it("switches to the same tab of another app", async () => {
    const user = userEvent.setup();
    at(`/apps/${notes.id}/retention`, "period=30d");
    renderWithClient(<AppHeader />);

    await expectSwitcher("Notes iOS");
    await user.click(await switcher());
    await user.type(await screen.findByPlaceholderText("Найти приложение…"), "maps");
    const options = screen.getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["Maps Android", "+ Добавить приложение"]);
    await user.click(await screen.findByRole("option", { name: "Maps Android" }));

    expect(nav.push).toHaveBeenCalledWith(`/apps/${maps.id}/retention?period=30d`);
  });

  it("lists apps by name and offers to add an app", async () => {
    const user = userEvent.setup();
    renderWithClient(<AppHeader />);

    await expectSwitcher("Все приложения");
    await user.click(await switcher());
    expect(await screen.findAllByRole("option")).toHaveLength(3);
    expect(screen.getAllByRole("option").map((o) => o.textContent)).toEqual([
      "Maps Android",
      "Notes iOS",
      "+ Добавить приложение",
    ]);
    await user.click(await screen.findByRole("option", { name: "+ Добавить приложение" }));

    expect(nav.push).toHaveBeenCalledWith("/apps/new");
  });

  it("keeps working when apps fail to load", async () => {
    let failed = false;
    server.use(
      http.get("*/v1/apps", () => {
        failed = true;
        return HttpResponse.json(errorBody("storage_unavailable"), { status: 503 });
      }),
    );
    at(`/apps/${notes.id}`);

    renderWithClient(<AppHeader />);

    await waitFor(() => expect(failed).toBe(true));
    await expectSwitcher("Все приложения");
    expect(within(mainNav()).getByRole("link", { name: "Настройки" })).toBeVisible();
  });

  it("logs out and goes to login", async () => {
    const user = userEvent.setup();
    let calls = 0;
    server.use(
      http.post("*/api/admin/v1/auth/logout", () => {
        calls++;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderWithClient(<AppHeader />);

    await user.click(screen.getByRole("button", { name: "Меню пользователя" }));
    await user.click(await screen.findByRole("menuitem", { name: "Выйти" }));

    await vi.waitFor(() => expect(nav.replace).toHaveBeenCalledWith("/login"));
    expect(calls).toBe(1);
  });
});

describe("AppHeader theme and language switches", () => {
  it("offers the theme toggle and the language switch next to the navigation", async () => {
    document.documentElement.dataset.theme = "light";

    renderWithClient(<AppHeader />);

    expect(await screen.findByRole("button", { name: "Включить тёмную тему" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Переключить на English" })).toHaveTextContent("EN");
  });

  it("marks the Russian docs as active inside the Russian documentation", () => {
    at("/ru/docs/recipes/curl");

    renderWithClient(<AppHeader />);

    expect(within(mainNav()).getByRole("link", { name: "Документация" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });
});

const english = ({ children }: { children: ReactNode }) => (
  <MessagesProvider locale="en" messages={getMessages("en")}>
    {children}
  </MessagesProvider>
);

describe("AppHeader in English", () => {
  it("speaks English and links the English docs", async () => {
    const user = userEvent.setup();
    document.documentElement.dataset.theme = "light";
    at("/docs/recipes/curl");
    renderWithClient(<AppHeader />, { wrapper: english });

    const navigation = screen.getByRole("navigation", { name: "Main navigation" });
    expect(within(navigation).getByRole("link", { name: "Portfolio" })).toHaveAttribute(
      "href",
      "/portfolio",
    );
    const docs = within(navigation).getByRole("link", { name: "Documentation" });
    expect(docs).toHaveAttribute("href", "/docs");
    expect(docs).toHaveAttribute("aria-current", "page");
    expect(within(navigation).getByRole("link", { name: "Settings" })).toBeVisible();
    expect(
      screen.getByRole("link", { name: "appmetrica.online — go to portfolio" }),
    ).toHaveAttribute("href", "/portfolio");
    expect(screen.getByRole("button", { name: "Switch to dark theme" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Switch to Русский" })).toHaveTextContent("RU");

    const appSwitcher = await screen.findByRole("combobox", { name: "App" });
    await waitFor(() => expect(appSwitcher).toHaveTextContent("All apps"));
    await user.click(appSwitcher);
    await user.type(await screen.findByPlaceholderText("Find an app…"), "zzz");
    expect(screen.getByText("Nothing found")).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "+ Add app" })).toBeInTheDocument();
  });

  it("offers to sign out or to leave the demo", async () => {
    const user = userEvent.setup();
    const { unmount } = renderWithClient(<AppHeader />, { wrapper: english });

    await user.click(screen.getByRole("button", { name: "User menu" }));
    expect(await screen.findByRole("menuitem", { name: "Sign out" })).toBeInTheDocument();
    unmount();

    renderWithClient(<AppHeader demo />, { wrapper: english });
    await user.click(screen.getByRole("button", { name: "User menu" }));
    expect(await screen.findByRole("menuitem", { name: "Exit demo" })).toHaveAttribute(
      "href",
      "/demo/exit",
    );
  });
});

describe("AppHeader in demo mode", () => {
  it("keeps the public docs link and offers to leave the demo", async () => {
    const user = userEvent.setup();
    renderWithClient(<AppHeader demo />);

    expect(within(mainNav()).getByRole("link", { name: "Документация" })).toHaveAttribute(
      "href",
      "/ru/docs",
    );
    expect(within(mainNav()).getByRole("link", { name: "Портфель" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Меню пользователя" }));
    expect(await screen.findByRole("menuitem", { name: "Выйти из демо" })).toHaveAttribute(
      "href",
      "/demo/exit",
    );
    expect(screen.queryByRole("menuitem", { name: "Выйти" })).not.toBeInTheDocument();
  });
});

// Полная навигация шапки занимает ~900 px: на планшете (768–1023) она не помещается, там тоже бургер.
describe("AppHeader on narrow screens", () => {
  it("switches to the full navigation only from lg", async () => {
    server.use(getListAppsMockHandler({ items: [notes, maps] }));
    at("/portfolio");
    renderWithClient(<AppHeader />);

    expect(screen.getByRole("button", { name: "Открыть меню" })).toHaveClass("lg:hidden");
    expect(screen.getByRole("button", { name: "Открыть меню" })).not.toHaveClass("md:hidden");
    expect(mainNav()).toHaveClass("hidden", "lg:block");
  });
});
