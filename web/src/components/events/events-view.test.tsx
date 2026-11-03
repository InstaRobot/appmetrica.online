import { QueryClientProvider } from "@tanstack/react-query";
import { screen, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EventList } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithLocale } from "@/test/rtl";
import { EventsView } from "./events-view";

const APP_ID = "0192aaaa-0000-7000-8000-000000000001";
const URL_PARAMS = "period=30d&country=KZ&env=development&unit=installs";

const nav = vi.hoisted(() => ({ search: "" }));
vi.mock("next/navigation", () => ({
  useParams: () => ({ appId: "0192aaaa-0000-7000-8000-000000000001" }),
  usePathname: () => "/apps/0192aaaa-0000-7000-8000-000000000001/events",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
  useSearchParams: () => new URLSearchParams(nav.search),
}));

const THREE: EventList = {
  truncated: false,
  events: [
    { name: "search_used", events: 2400, people: 600, prev_events: 3000, prev_people: 700 },
    { name: "note_created", events: 9000, people: 1500, prev_events: 7500, prev_people: 1400 },
    { name: "purchase", events: 40, people: 40, prev_events: 0, prev_people: 0 },
  ],
};

let lastQuery: URLSearchParams | undefined;

function mockApi(list: EventList | { status: number }) {
  lastQuery = undefined;
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(makeApp({ id: APP_ID, timezone: "Europe/Moscow", platforms: ["ios"] })),
    ),
    http.get(`*/v1/apps/${APP_ID}/events`, ({ request }) => {
      lastQuery = new URL(request.url).searchParams;
      return "status" in list
        ? HttpResponse.json(
            { error: { code: "internal_error", message: "boom", request_id: "r1" } },
            { status: list.status },
          )
        : HttpResponse.json(list);
    }),
    http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, () =>
      HttpResponse.json({ dimension: "platform", metric: "active", total: 0, rows: [] }),
    ),
  );
}

function renderView(search = URL_PARAMS) {
  nav.search = search;
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return renderWithLocale(
    <QueryClientProvider client={client}>
      <EventsView />
    </QueryClientProvider>,
    "en",
    { wrapper: withNuqsTestingAdapter({ searchParams: search, hasMemory: true }) },
  );
}

beforeEach(() => {
  vi.useFakeTimers({ now: new Date("2026-10-13T09:00:00Z"), shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
});

describe("EventsView", () => {
  it("lists three events by descending count with links carrying the URL parameters", async () => {
    mockApi(THREE);
    renderView();

    const table = await screen.findByRole("table", { name: "Custom events" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(3);
    const links = rows.map((r) => within(r).getByRole("link"));
    expect(links.map((l) => l.textContent)).toEqual(["note_created", "search_used", "purchase"]);
    for (const link of links) {
      const [path, query] = (link.getAttribute("href") ?? "").split("?");
      expect(path).toBe(`/apps/${APP_ID}/events/${link.textContent}`);
      expect(Object.fromEntries(new URLSearchParams(query))).toEqual({
        period: "30d",
        country: "KZ",
        env: "development",
        unit: "installs",
      });
    }
    // Берётся запрос с периодом, фильтрами, окружением, unit и сравнением с прошлым периодом.
    expect(lastQuery?.get("compare")).toBe("previous_period");
    expect(lastQuery?.get("country")).toBe("KZ");
    expect(lastQuery?.get("environment")).toBe("development");
    expect(lastQuery?.get("unit")).toBe("installs");
  });

  it("shows events, people, the change to the previous period and events per person", async () => {
    mockApi(THREE);
    renderView();

    const table = await screen.findByRole("table", { name: "Custom events" });
    const cells = (row: HTMLElement) =>
      within(row)
        .getAllByRole("cell")
        .map((c) => c.textContent);
    const rows = within(table).getAllByRole("row").slice(1);
    // Колонки: имя, события, люди, изменение, на человека.
    expect(cells(rows[0] as HTMLElement)).toEqual(["note_created", "9,000", "1,500", "+20%", "6"]);
    expect(cells(rows[1] as HTMLElement)).toEqual(["search_used", "2,400", "600", "−20%", "4"]);
    // Без событий в прошлом периоде сравнивать не с чем.
    expect(cells(rows[2] as HTMLElement)).toEqual(["purchase", "40", "40", "—", "1"]);
  });

  it("renders a row without people and without a previous period as 0 and a dash", async () => {
    mockApi({
      truncated: false,
      events: [{ name: "ghost_event", events: 5, people: 0, prev_events: 0, prev_people: 0 }],
    });
    renderView();

    const table = await screen.findByRole("table", { name: "Custom events" });
    const row = within(table).getAllByRole("row")[1] as HTMLElement;
    expect(
      within(row)
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ).toEqual(["ghost_event", "5", "0", "—", "0"]);
    expect(row.textContent).not.toMatch(/NaN|Infinity/);
  });

  it("monospaces the event name and does not translate it", async () => {
    mockApi(THREE);
    renderView();

    const link = await screen.findByRole("link", { name: "note_created" });
    expect(link.className).toContain("font-mono");
  });

  it("shows the empty state with a link to the events docs", async () => {
    mockApi({ events: [], truncated: false });
    renderView();

    expect(await screen.findByText("No custom events yet")).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    const link = screen.getByRole("link", { name: "How to send events" });
    expect(link).toHaveAttribute("href", "/docs/concepts/events");
  });

  it("tells when the list was cut at the server limit", async () => {
    mockApi({ ...THREE, truncated: true });
    renderView();

    expect(await screen.findByText(/Only the first 500 events are shown/)).toBeInTheDocument();
  });

  it("shows an error with a retry instead of the table", async () => {
    mockApi({ status: 500 });
    renderView();

    expect(await screen.findByRole("button", { name: "Try again" })).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});
