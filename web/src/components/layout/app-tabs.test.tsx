import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { AppTabs, reportQuery } from "./app-tabs";

const APP_ID = "0192aaaa-0000-7000-8000-000000000001";

const nav = vi.hoisted(() => ({ pathname: "/", search: "" }));
vi.mock("next/navigation", () => ({
  usePathname: () => nav.pathname,
  useSearchParams: () => new URLSearchParams(nav.search),
}));

const tab = (name: string) =>
  within(screen.getByRole("navigation", { name: "Разделы приложения" })).getByRole("link", {
    name,
  });

beforeEach(() => {
  nav.pathname = `/apps/${APP_ID}`;
  nav.search = "";
});

describe("AppTabs", () => {
  it("keeps the platform on tab change", () => {
    nav.search = "platform=ios";
    render(<AppTabs appId={APP_ID} />);

    expect(tab("Retention")).toHaveAttribute("href", `/apps/${APP_ID}/retention?platform=ios`);
    expect(tab("Realtime")).toHaveAttribute("href", `/apps/${APP_ID}/realtime?platform=ios`);
    expect(tab("Профили")).toHaveAttribute("href", `/apps/${APP_ID}/profiles?platform=ios`);
  });

  it("keeps the period, filters, environment and unit when going from Audience to Events", () => {
    nav.search = "period=30d&country=KZ&app_version=3.5.0&env=development&unit=installs&metric=new";
    render(<AppTabs appId={APP_ID} />);

    const href = tab("События").getAttribute("href") ?? "";
    expect(href.startsWith(`/apps/${APP_ID}/events?`)).toBe(true);
    const kept = new URLSearchParams(href.split("?")[1]);
    expect(Object.fromEntries(kept)).toEqual({
      period: "30d",
      country: "KZ",
      app_version: "3.5.0",
      env: "development",
      unit: "installs",
    });
  });

  it("marks Events active on the page of one event", () => {
    nav.pathname = `/apps/${APP_ID}/events/note_created`;
    render(<AppTabs appId={APP_ID} />);

    expect(tab("События")).toHaveAttribute("aria-current", "page");
  });

  it("names the tabs in English", () => {
    renderWithLocale(<AppTabs appId={APP_ID} />, "en");

    const tabs = within(screen.getByRole("navigation", { name: "App sections" })).getAllByRole(
      "link",
    );
    expect(tabs.map((t) => t.textContent)).toEqual([
      "Audience",
      "Retention",
      "Events",
      "Realtime",
      "Profiles",
      "Integration",
      "Settings",
    ]);
    expect(tabs[0]).toHaveAttribute("aria-current", "page");
  });

  it("drops parameters of a single report", () => {
    expect(
      reportQuery(new URLSearchParams("platform=android&q=u%3A42&metric=new&cohort=week")),
    ).toBe("?platform=android");
    expect(reportQuery(new URLSearchParams("q=u%3A42"))).toBe("");
  });

  it("scrolls the active tab into view: on a phone the tabs scroll sideways", () => {
    nav.pathname = `/apps/${APP_ID}/integration`;
    const scroll = vi.spyOn(Element.prototype, "scrollIntoView");
    render(<AppTabs appId={APP_ID} />);

    expect(scroll).toHaveBeenCalledWith({ block: "nearest", inline: "nearest" });
    expect(scroll.mock.contexts[0]).toBe(tab("Интеграция"));
  });
});
