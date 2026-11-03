import { QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { type OnUrlUpdateFunction, withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EventBreakdown, EventReport } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithLocale } from "@/test/rtl";
import { EventView } from "./event-view";

// Таблица разбивки — после двух запросов подряд (отчёт события, затем разбивка по его первому ключу)
// и ленивой загрузки: секунды по умолчанию у findBy* на раннере CI не хватало (как в lazy-calendar.test.tsx).
const BREAKDOWN_WAIT = { timeout: 5000 };

const APP_ID = "0192aaaa-0000-7000-8000-000000000001";
const URL_PARAMS = "period=30d&country=KZ&env=development&unit=installs";

const nav = vi.hoisted(() => ({ search: "" }));
vi.mock("next/navigation", () => ({
  useParams: () => ({
    appId: "0192aaaa-0000-7000-8000-000000000001",
    eventName: "note_created",
  }),
  usePathname: () => "/apps/0192aaaa-0000-7000-8000-000000000001/events/note_created",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
  useSearchParams: () => new URLSearchParams(nav.search),
}));

const REPORT: EventReport = {
  name: "note_created",
  summary: {
    events: 1200,
    people: 300,
    events_per_person: 4,
    prev_events: 1000,
    prev_people: 400,
    prev_events_per_person: 2.5,
  },
  timeseries: [
    { bucket: "2026-10-12T00:00:00+03:00", events: 500, people: 200 },
    { bucket: "2026-10-13T00:00:00+03:00", events: 700, people: 250 },
  ],
  property_keys: [
    { key: "plan", events: 1100 },
    { key: "source", events: 900 },
  ],
};

// 1 + 1 + 1 из 3 событий: ровно по трети, доли 33,3 % × 3 дают 99,9 % без поправки округления.
const THIRDS: EventBreakdown = {
  property: "plan",
  values: [
    { value: "pro", events: 1, people: 1 },
    { value: "Free Plan", events: 1, people: 1 },
  ],
  missing: { events: 1, people: 1 },
};

const SOURCE: EventBreakdown = {
  property: "source",
  values: [
    { value: "keyboard", events: 600, people: 150 },
    { value: "voice", events: 200, people: 80 },
  ],
  other: { events: 60, people: 30 },
  missing: { events: 140, people: 50 },
};

type Seen = { report?: URLSearchParams; breakdown: URLSearchParams[] };
let seen: Seen;

function mockApi(opts: {
  report?: EventReport | { status: number; code: string };
  breakdowns?: Record<string, EventBreakdown>;
}) {
  seen = { breakdown: [] };
  const fail = (status: number, code: string) =>
    HttpResponse.json({ error: { code, message: "x", request_id: "r1" } }, { status });
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(makeApp({ id: APP_ID, timezone: "Europe/Moscow", platforms: ["ios"] })),
    ),
    http.get(`*/v1/apps/${APP_ID}/events/note_created`, ({ request }) => {
      seen.report = new URL(request.url).searchParams;
      const r = opts.report ?? REPORT;
      return "status" in r ? fail(r.status, r.code) : HttpResponse.json(r);
    }),
    http.get(`*/v1/apps/${APP_ID}/events/note_created/breakdown`, ({ request }) => {
      const q = new URL(request.url).searchParams;
      seen.breakdown.push(q);
      const b = opts.breakdowns?.[q.get("property") ?? ""];
      return b ? HttpResponse.json(b) : fail(400, "invalid_request");
    }),
    http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, () =>
      HttpResponse.json({ dimension: "platform", metric: "active", total: 0, rows: [] }),
    ),
  );
}

function renderView(search = URL_PARAMS, onUrlUpdate?: OnUrlUpdateFunction) {
  nav.search = search;
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return renderWithLocale(
    <QueryClientProvider client={client}>
      <EventView />
    </QueryClientProvider>,
    "en",
    {
      wrapper: withNuqsTestingAdapter({
        searchParams: search,
        hasMemory: true,
        ...(onUrlUpdate ? { onUrlUpdate } : {}),
      }),
    },
  );
}

beforeEach(() => {
  vi.useFakeTimers({ now: new Date("2026-10-13T09:00:00Z"), shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
});

const percent = (cell: HTMLElement | undefined) =>
  Number.parseFloat((cell?.textContent ?? "").replace("%", ""));

describe("EventView", () => {
  it("shows events, people and per person with the change to the previous period", async () => {
    mockApi({ breakdowns: { plan: SOURCE } });
    renderView();

    const totals = await screen.findByRole("region", { name: "Event totals" });
    await waitFor(() => expect(within(totals).getByText("1,200")).toBeInTheDocument());
    const text = totals.textContent ?? "";
    expect(text).toContain("Events");
    expect(text).toContain("Installs");
    expect(text).toContain("Events per person");
    expect(within(totals).getByText("300")).toBeInTheDocument();
    expect(within(totals).getByText("4")).toBeInTheDocument();
    // 1000 → 1200 события, 400 → 300 людей, 2,5 → 4 на человека.
    expect(within(totals).getByText("+20%")).toBeInTheDocument();
    expect(within(totals).getByText("−25%")).toBeInTheDocument();
    expect(within(totals).getByText("+60%")).toBeInTheDocument();
    expect(seen.report?.get("compare")).toBe("previous_period");
    expect(seen.report?.get("granularity")).toBe("day");
    expect(seen.report?.get("environment")).toBe("development");
    expect(seen.report?.get("country")).toBe("KZ");
  });

  it("monospaces the event name and links back to the list keeping the parameters", async () => {
    mockApi({ breakdowns: { plan: SOURCE } });
    renderView();

    const heading = await screen.findByRole("heading", { level: 1, name: "note_created" });
    expect(heading.className).toContain("font-mono");
    const back = screen.getByRole("link", { name: /Events/ });
    const [path, query] = (back.getAttribute("href") ?? "").split("?");
    expect(path).toBe(`/apps/${APP_ID}/events`);
    expect(Object.fromEntries(new URLSearchParams(query))).toEqual({
      period: "30d",
      country: "KZ",
      env: "development",
      unit: "installs",
    });
  });

  it("breaks down by the first property key without writing it into the URL", async () => {
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    mockApi({ breakdowns: { plan: THIRDS } });
    renderView(URL_PARAMS, onUrlUpdate);

    const table = await screen.findByRole(
      "table",
      { name: "note_created by plan" },
      BREAKDOWN_WAIT,
    );
    expect(within(table).getByText("pro")).toBeInTheDocument();
    expect(seen.breakdown[0]?.get("property")).toBe("plan");
    expect(seen.breakdown[0]?.get("limit")).toBe("20");
    expect(onUrlUpdate).not.toHaveBeenCalled();
  });

  // AC-009.1
  it("writes the chosen property into the URL and shows values with shares summing to 100%", async () => {
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mockApi({ breakdowns: { plan: THIRDS, source: SOURCE } });
    renderView(URL_PARAMS, onUrlUpdate);

    await screen.findByRole("table", { name: "note_created by plan" }, BREAKDOWN_WAIT);
    await user.click(await screen.findByRole("combobox", { name: "Property" }));
    await user.click(await screen.findByRole("option", { name: "source" }));

    expect(onUrlUpdate.mock.lastCall?.[0].queryString).toContain("property=source");
    const table = await screen.findByRole(
      "table",
      { name: "note_created by source" },
      BREAKDOWN_WAIT,
    );
    const rows = within(table).getAllByRole("row").slice(1);
    const cells = (row: HTMLElement) =>
      within(row)
        .getAllByRole("cell")
        .map((c) => c.textContent);
    // Колонки: значение, события, люди (unit=installs), доля. Значения — как отправлены.
    expect(cells(rows[0] as HTMLElement)).toEqual(["keyboard", "600", "150", "60%"]);
    expect(cells(rows[1] as HTMLElement)).toEqual(["voice", "200", "80", "20%"]);
    expect(cells(rows[2] as HTMLElement)).toEqual(["Other", "60", "30", "6%"]);
    expect(cells(rows[3] as HTMLElement)).toEqual(["No property", "140", "50", "14%"]);
    const total = rows.reduce((s, r) => s + percent(within(r).getAllByRole("cell")[3]), 0);
    expect(total).toBeCloseTo(100, 5);
    expect(seen.breakdown.at(-1)?.get("property")).toBe("source");
  });

  it("makes the displayed shares add up to 100% when the exact thirds would not", async () => {
    mockApi({ breakdowns: { plan: THIRDS } });
    renderView();

    const table = await screen.findByRole(
      "table",
      { name: "note_created by plan" },
      BREAKDOWN_WAIT,
    );
    const rows = within(table).getAllByRole("row").slice(1);
    const shares = rows.map((r) => percent(within(r).getAllByRole("cell")[3]));
    expect(shares).toHaveLength(3);
    // Наибольший остаток: лишняя десятая достаётся первой строке, 33,4 + 33,3 + 33,3.
    expect(shares).toEqual([33.4, 33.3, 33.3]);
    expect(shares.reduce((s, x) => s + x, 0)).toBeCloseTo(100, 5);
    // Значение свойства не переводится и не меняет регистр.
    expect(within(table).getByText("Free Plan")).toBeInTheDocument();
  });

  it("shows no Other row when everything fits and no missing row when every event has the key", async () => {
    mockApi({
      breakdowns: {
        plan: {
          property: "plan",
          values: [{ value: "pro", events: 10, people: 5 }],
          missing: { events: 0, people: 0 },
        },
      },
    });
    renderView();

    const table = await screen.findByRole(
      "table",
      { name: "note_created by plan" },
      BREAKDOWN_WAIT,
    );
    expect(within(table).queryByText("Other")).not.toBeInTheDocument();
    expect(within(table).queryByText("No property")).not.toBeInTheDocument();
    expect(within(table).getByText("100%")).toBeInTheDocument();
  });

  it("labels a present-but-empty value with a muted placeholder, distinct from the missing row", async () => {
    mockApi({
      breakdowns: {
        plan: {
          property: "plan",
          values: [
            { value: "pro", events: 6, people: 3 },
            { value: "", events: 3, people: 2 },
          ],
          missing: { events: 1, people: 1 },
        },
      },
    });
    renderView();

    const table = await screen.findByRole(
      "table",
      { name: "note_created by plan" },
      BREAKDOWN_WAIT,
    );
    const placeholder = within(table).getByText("empty value");
    expect(placeholder.className).toContain("text-fg-muted");
    expect(placeholder.className).not.toContain("font-mono");
    expect(within(table).getByText("No property")).toBeInTheDocument();
    expect(within(table).getAllByRole("row")).toHaveLength(4);
  });

  it("takes the property and the limit from the URL", async () => {
    mockApi({ breakdowns: { source: SOURCE } });
    renderView(`${URL_PARAMS}&property=source&limit=50`);

    await screen.findByRole("table", { name: "note_created by source" }, BREAKDOWN_WAIT);
    expect(seen.breakdown[0]?.get("property")).toBe("source");
    expect(seen.breakdown[0]?.get("limit")).toBe("50");
  });

  it("switches the chart between events and people", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mockApi({ breakdowns: { plan: SOURCE } });
    renderView();

    const group = await screen.findByRole("radiogroup", { name: "Chart metric" });
    const events = within(group).getByRole("radio", { name: "Events" });
    const people = within(group).getByRole("radio", { name: "Installs" });
    expect(events).toBeChecked();
    await user.click(people);
    expect(people).toBeChecked();
  });

  // AC-009.2
  it("shows Event not found with a link to the list when the server says event_not_found", async () => {
    mockApi({ report: { status: 404, code: "event_not_found" } });
    renderView();

    expect(await screen.findByText("Event not found")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Back to events" });
    const [path, query] = (link.getAttribute("href") ?? "").split("?");
    expect(path).toBe(`/apps/${APP_ID}/events`);
    expect(new URLSearchParams(query).get("period")).toBe("30d");
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("shows an error with a retry for other failures", async () => {
    mockApi({ report: { status: 500, code: "internal_error" } });
    renderView();

    expect(await screen.findByRole("button", { name: "Try again" })).toBeInTheDocument();
    expect(screen.queryByText("Event not found")).not.toBeInTheDocument();
  });

  it("explains range_too_large under the period picker instead of an error state", async () => {
    mockApi({ report: { status: 400, code: "range_too_large" } });
    renderView();

    expect(await screen.findByText(/period is too long|too large|too long/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
  });

  it("renders zeros, not an error, for an event without events in the period", async () => {
    mockApi({
      report: {
        name: "note_created",
        summary: { events: 0, people: 0, events_per_person: 0 },
        timeseries: [{ bucket: "2026-10-13T00:00:00+03:00", events: 0, people: 0 }],
        property_keys: [],
      },
    });
    renderView();

    const totals = await screen.findByRole("region", { name: "Event totals" });
    await waitFor(() => expect(within(totals).getAllByText("0").length).toBe(3));
    expect(screen.getByText("This event has no properties in the period.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
    // Без свойств разбивка не запрашивается.
    expect(seen.breakdown).toHaveLength(0);
  });
});
