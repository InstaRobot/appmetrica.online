import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RecentEvent } from "@/lib/api/gen/model";
import { EventFeed } from "./event-feed";

const APP_ID = "0192aaaa-0000-7000-8000-000000000007";

function event(n: number, overrides: Partial<RecentEvent> = {}): RecentEvent {
  const id = String(n).padStart(12, "0");
  return {
    insert_id: `0192bbbb-0000-7000-8000-${id}`,
    event_time: `2026-10-13T11:32:0${n}Z`,
    received_at: `2026-10-13T11:32:0${n}Z`,
    name: "session_start",
    environment: "production",
    platform: "ios",
    app_version: "2.1.1",
    os_name: "iOS",
    os_version: "26.1",
    device_model: "iPhone17,1",
    country: "RU",
    city: "Москва",
    install_id: `5d8e0000-0000-4000-8000-${id}`,
    person_key: `i:5d8e0000-0000-4000-8000-${id}`,
    ...overrides,
  };
}

function mockReducedMotion(reduce: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: reduce && query === "(prefers-reduced-motion: reduce)",
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

const feed = () => screen.getByRole("table", { name: "Последние события" });
const bodyRows = () => within(feed()).getAllByRole("row").slice(1);

describe("EventFeed", () => {
  it("renders events with short ids linking to profiles", () => {
    render(
      <EventFeed
        appId={APP_ID}
        timezone="Europe/Moscow"
        events={[
          event(4),
          event(1, {
            name: "session_end",
            platform: "android",
            app_version: "2.0.9",
            country: "DE",
            city: "Berlin",
            install_id: "a91c0000-0000-4000-8000-0000000077d0",
            person_key: "u:42",
          }),
        ]}
      />,
    );

    const headers = within(feed())
      .getAllByRole("columnheader")
      .map((h) => h.textContent);
    expect(headers).toEqual([
      "Время",
      "Событие",
      "Платформа",
      "Версия",
      "Страна / город",
      "Install",
      "Человек",
    ]);
    const [first, second] = bodyRows() as [HTMLElement, HTMLElement];
    expect(first).toHaveTextContent("14:32:04");
    expect(first).toHaveTextContent("session_start");
    expect(first).toHaveTextContent("iOS");
    expect(first).toHaveTextContent("RU · Москва");
    expect(second).toHaveTextContent("Android");
    expect(second).toHaveTextContent("2.0.9");
    expect(second).toHaveTextContent("DE · Berlin");

    const install = within(second).getByRole("link", {
      name: "Установка a91c0000-0000-4000-8000-0000000077d0",
    });
    expect(install).toHaveTextContent("a91c…77d0");
    expect(install).toHaveAttribute("title", "a91c0000-0000-4000-8000-0000000077d0");
    expect(install).toHaveAttribute("href", `/apps/${APP_ID}/profiles/u%3A42`);
    const person = within(second).getByRole("link", { name: "Профиль u:42" });
    expect(person).toHaveTextContent("u:42");
    expect(person).toHaveAttribute("href", `/apps/${APP_ID}/profiles/u%3A42`);

    const longPerson = within(first).getByRole("link", {
      name: "Профиль i:5d8e0000-0000-4000-8000-000000000004",
    });
    expect(longPerson).toHaveTextContent("i:5d…0004");
  });

  it("shows a dash for unknown country and only the country without a city", () => {
    render(
      <EventFeed
        appId={APP_ID}
        timezone="Europe/Moscow"
        events={[event(1, { country: "", city: "" }), event(2, { city: "" })]}
      />,
    );
    const [first, second] = bodyRows() as [HTMLElement, HTMLElement];
    expect(within(first).getAllByRole("cell")[4]).toHaveTextContent("—");
    expect(within(second).getAllByRole("cell")[4]).toHaveTextContent(/^RU$/);
  });

  it("highlights only rows that were not in the previous response", () => {
    mockReducedMotion(false);
    const { rerender } = render(
      <EventFeed appId={APP_ID} timezone="Europe/Moscow" events={[event(2), event(1)]} />,
    );
    expect(feed().querySelectorAll("[data-new]")).toHaveLength(0);

    rerender(
      <EventFeed appId={APP_ID} timezone="Europe/Moscow" events={[event(3), event(2), event(1)]} />,
    );
    const highlighted = feed().querySelectorAll("tr[data-new]");
    expect(highlighted).toHaveLength(1);
    expect(highlighted[0]).toHaveTextContent("14:32:03");
    expect(highlighted[0]).toHaveClass("animate-row-highlight");

    // Тот же набор событий новым массивом — подсветки нет.
    rerender(
      <EventFeed appId={APP_ID} timezone="Europe/Moscow" events={[event(3), event(2), event(1)]} />,
    );
    expect(feed().querySelectorAll("tr[data-new]")).toHaveLength(0);
  });

  it("does not highlight new rows with reduced motion", () => {
    mockReducedMotion(true);
    const { rerender } = render(
      <EventFeed appId={APP_ID} timezone="Europe/Moscow" events={[event(1)]} />,
    );
    rerender(<EventFeed appId={APP_ID} timezone="Europe/Moscow" events={[event(2), event(1)]} />);

    expect(bodyRows()).toHaveLength(2);
    expect(feed().querySelectorAll("tr[data-new]")).toHaveLength(0);
    expect(feed().querySelector(".animate-row-highlight")).toBeNull();
  });
});
