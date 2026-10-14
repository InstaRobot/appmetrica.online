import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
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

  it("drops parameters of a single report", () => {
    expect(
      reportQuery(new URLSearchParams("platform=android&q=u%3A42&metric=new&cohort=week")),
    ).toBe("?platform=android");
    expect(reportQuery(new URLSearchParams("q=u%3A42"))).toBe("");
  });
});
