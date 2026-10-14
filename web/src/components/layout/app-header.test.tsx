import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getListAppsMockHandler } from "@/lib/api/gen/endpoints/apps/apps.msw";
import { errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { AppHeader } from "./app-header";
import { AppTabs } from "./app-tabs";

const nav = vi.hoisted(() => ({
  pathname: "/",
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
  at("/");
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
      "/docs",
    );
    await expectSwitcher("Все приложения");
    expect(screen.getByRole("link", { name: "appmetrica.online — в портфель" })).toHaveAttribute(
      "href",
      "/",
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

describe("AppHeader theme switch", () => {
  it("offers the light/dark theme toggle next to the navigation", async () => {
    document.documentElement.dataset.theme = "light";

    renderWithClient(<AppHeader />);

    expect(await screen.findByRole("button", { name: "Включить тёмную тему" })).toBeInTheDocument();
  });
});
