import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { type OnUrlUpdateFunction, withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PortfolioView } from "@/app/(dashboard)/portfolio-view";
import { getGetSettingsMockHandler } from "@/lib/api/gen/endpoints/admin-settings/admin-settings.msw";
import { getListAppsMockHandler } from "@/lib/api/gen/endpoints/apps/apps.msw";
import type { Portfolio, PortfolioRow } from "@/lib/api/gen/model";
import { errorBody, app as makeApp, portfolioRow } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { type PortfolioSort, PortfolioTable } from "./portfolio-table";

// Testing Library сводит пробельные символы текста узла (и U+00A0) к обычному пробелу.
const NBSP = " ";
const MINUS = "−";

const nav = vi.hoisted(() => ({ push: vi.fn<(href: string) => void>() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: nav.push, replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => "/portfolio",
  useSearchParams: () => new URLSearchParams(),
}));

const notes = portfolioRow({
  app: {
    id: "0192aaaa-0000-7000-8000-000000000001",
    name: "Notes iOS",
    platform: "ios",
    platforms: ["ios"],
    status: "active",
  },
  active_today: 8912,
  active_date: 12480,
  active_delta_pct: 4.1,
  sparkline: Array.from({ length: 30 }, (_, i) => ({
    day: `2026-09-${i + 1}`,
    active: 10000 + i * 100,
  })),
});
const maps = portfolioRow({
  app: {
    id: "0192aaaa-0000-7000-8000-000000000002",
    name: "Maps Android",
    platform: "android",
    platforms: ["android"],
    status: "active",
  },
  active_today: 6001,
  active_date: 8902,
  active_delta_pct: -1.2,
  retention_d1: 0.31,
  retention_d7: 0.11,
  sparkline: Array.from({ length: 30 }, (_, i) => ({
    day: `2026-09-${i + 1}`,
    active: 9000 - i * 50,
  })),
});
const site = portfolioRow({
  app: {
    id: "0192aaaa-0000-7000-8000-000000000003",
    name: "site.ru",
    platform: "web",
    platforms: ["web"],
    status: "active",
  },
  active_today: 940,
  active_date: 3117,
  active_delta_pct: null,
  retention_d1: null,
  retention_d7: null,
});

function portfolio(rows: PortfolioRow[], overrides: Partial<Portfolio> = {}): Portfolio {
  return { date: "2026-10-12", environment: "production", unit: "persons", rows, ...overrides };
}

type Captured = { portfolio: URLSearchParams[] };

function mockApi(rows: PortfolioRow[], apps = rows.map((r) => makeApp({ ...r.app }))): Captured {
  const captured: Captured = { portfolio: [] };
  server.use(
    http.get("*/v1/portfolio", ({ request }) => {
      const params = new URL(request.url).searchParams;
      captured.portfolio.push(params);
      return HttpResponse.json(portfolio(rows, { date: params.get("date") ?? "2026-10-12" }));
    }),
    getListAppsMockHandler({ items: apps }),
    getGetSettingsMockHandler({
      owner_timezone: "Europe/Moscow",
      telegram_chat_id: null,
      digest_enabled: false,
      digest_time: "09:00",
      alerts_enabled: false,
      updated_at: "2026-10-01T00:00:00Z",
    }),
  );
  return captured;
}

function renderView(searchParams = "", onUrlUpdate?: OnUrlUpdateFunction) {
  return renderWithClient(<PortfolioView />, {
    // hasMemory: адаптер хранит обновления URL, как браузер, — иначе состояние откатывается к searchParams.
    wrapper: withNuqsTestingAdapter({
      searchParams,
      hasMemory: true,
      ...(onUrlUpdate ? { onUrlUpdate } : {}),
    }),
  });
}

const table = () => screen.findByRole("table", { name: /^Портфель приложений за/ });
const bodyRows = (t: HTMLElement) => within(t).getAllByRole("row").slice(1);
const names = (t: HTMLElement) =>
  bodyRows(t).map((r) => within(r).getAllByRole("cell")[0]?.textContent);
const rowOf = (t: HTMLElement, name: string) => {
  const row = bodyRows(t).find((r) => r.textContent?.includes(name));
  if (!row) throw new Error(`row ${name} not found`);
  return row;
};

beforeEach(() => {
  nav.push.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("PortfolioView", () => {
  it("sorts by DAU and shows signed delta", async () => {
    mockApi([site, notes, maps]);
    const user = userEvent.setup();
    renderView("?sort=name");

    const t = await table();
    expect(names(t)).toEqual(["Maps Android", "Notes iOS", "site.ru"]);

    await user.click(within(t).getByRole("button", { name: /^DAU 12\sокт/ }));

    expect(names(t)).toEqual(["Notes iOS", "Maps Android", "site.ru"]);
    const dau = within(t).getByRole("columnheader", { name: /^DAU 12\sокт/ });
    expect(dau).toHaveAttribute("aria-sort", "descending");
    expect(within(t).getByRole("columnheader", { name: /^Приложение/ })).toHaveAttribute(
      "aria-sort",
      "none",
    );

    const up = within(rowOf(t, "Notes iOS")).getByText(`+4,1${NBSP}%`);
    expect(up).toHaveClass("text-positive");
    const down = within(rowOf(t, "Maps Android")).getByText(`${MINUS}1,2${NBSP}%`);
    expect(down).toHaveClass("text-negative");
    expect(within(rowOf(t, "Notes iOS")).getByText(`12${NBSP}480`)).toBeInTheDocument();
  });

  it("shows empty state", async () => {
    mockApi([], []);
    renderView();

    expect(await screen.findByRole("heading", { name: "Пока нет приложений" })).toBeInTheDocument();
    expect(
      screen.getByText(
        "Добавьте первое приложение — получите ключ и готовые сниппеты для отправки событий.",
      ),
    ).toBeInTheDocument();
    const links = screen.getAllByRole("link", { name: "Добавить приложение" });
    expect(links.length).toBeGreaterThan(0);
    for (const link of links) expect(link).toHaveAttribute("href", "/apps/new");
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("titles columns with the report date and the same weekday a week earlier", async () => {
    mockApi([notes]);
    renderView();

    const t = await table();
    expect(t).toHaveAccessibleName(/^Портфель приложений за 12\sокт\s2026$/);
    const headers = within(t)
      .getAllByRole("columnheader")
      .map((h) => h.textContent);
    expect(headers).toEqual([
      "Приложение",
      "Платформы",
      "Сегодня",
      "DAU 12\u00a0окт",
      "Δ к пн\u00a05\u00a0окт",
      "Новые",
      "Сессии",
      "D1",
      "D7",
      "30 дней",
    ]);
  });

  it("renders missing values as dashes and keeps them last in either direction", async () => {
    mockApi([site, notes, maps]);
    const user = userEvent.setup();
    renderView();

    const t = await table();
    const cells = within(rowOf(t, "site.ru")).getAllByRole("cell");
    expect(cells[4]).toHaveTextContent("—");
    expect(cells[7]).toHaveTextContent("—");
    expect(cells[8]).toHaveTextContent("—");
    expect(within(rowOf(t, "Notes iOS")).getAllByRole("cell")[7]).toHaveTextContent(`38${NBSP}%`);

    const d1 = within(t).getByRole("button", { name: /^D1/ });
    await user.click(d1);
    expect(names(t)).toEqual(["Notes iOS", "Maps Android", "site.ru"]);
    await user.click(d1);
    expect(within(t).getByRole("columnheader", { name: /^D1/ })).toHaveAttribute(
      "aria-sort",
      "ascending",
    );
    expect(names(t)).toEqual(["Maps Android", "Notes iOS", "site.ru"]);
  });

  it("writes the sort to the URL and drops it for the default order", async () => {
    mockApi([site, notes, maps]);
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup();
    renderView("", onUrlUpdate);

    const t = await table();
    expect(names(t)).toEqual(["Notes iOS", "Maps Android", "site.ru"]);

    await user.click(within(t).getByRole("button", { name: /^Приложение/ }));
    expect(names(t)).toEqual(["Maps Android", "Notes iOS", "site.ru"]);
    expect(onUrlUpdate.mock.lastCall?.[0].searchParams.get("sort")).toBe("name");

    await user.click(within(t).getByRole("button", { name: /^DAU/ }));
    expect(onUrlUpdate.mock.lastCall?.[0].searchParams.has("sort")).toBe(false);
  });

  it("opens the app from its row and links the name", async () => {
    mockApi([notes]);
    const user = userEvent.setup();
    renderView();

    const t = await table();
    const row = rowOf(t, "Notes iOS");
    expect(within(row).getByRole("link", { name: "Notes iOS" })).toHaveAttribute(
      "href",
      `/apps/${notes.app.id}`,
    );
    await user.click(within(row).getByText(`12${NBSP}480`));
    expect(nav.push).toHaveBeenCalledWith(`/apps/${notes.app.id}`);
  });

  it("describes the 30-day trend for screen readers", async () => {
    mockApi([notes, maps]);
    renderView();

    const t = await table();
    expect(within(rowOf(t, "Notes iOS")).getByText("Тренд за 30 дней: рост")).toBeInTheDocument();
    expect(
      within(rowOf(t, "Maps Android")).getByText("Тренд за 30 дней: падение"),
    ).toBeInTheDocument();
  });

  it("warns when no app has events for the date", async () => {
    const zero = portfolioRow({
      ...notes,
      active_today: 0,
      active_date: 0,
      new_date: 0,
      sessions_date: 0,
      active_delta_pct: null,
      retention_d1: null,
      retention_d7: null,
    });
    mockApi([zero]);
    renderView();

    await table();
    expect(
      screen.getByText("За 12 окт 2026 событий нет. Проверьте вкладку «Интеграция» у приложений."),
    ).toBeInTheDocument();
  });

  it("shows a skeleton while loading", async () => {
    mockApi([notes]);
    renderView();

    expect(screen.getByRole("status", { name: "Загрузка…" })).toBeInTheDocument();
    await table();
    expect(screen.queryByRole("status", { name: "Загрузка…" })).not.toBeInTheDocument();
  });

  it("shows an inline error and retries", async () => {
    mockApi([notes]);
    let calls = 0;
    server.use(
      http.get("*/v1/portfolio", () => {
        calls += 1;
        if (calls === 1) {
          return HttpResponse.json(errorBody("internal_error", "boom", "01JREQ"), { status: 500 });
        }
        return HttpResponse.json(portfolio([notes]));
      }),
    );
    const user = userEvent.setup();
    renderView();

    expect(await screen.findByText("Ошибка сервера. Повторите через минуту.")).toBeInTheDocument();
    expect(screen.getByText("Код запроса: 01JREQ")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await table()).toBeInTheDocument();
  });

  it("switches environment through the URL", async () => {
    const captured = mockApi([notes]);
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup();
    renderView("", onUrlUpdate);

    await table();
    expect(captured.portfolio.at(-1)?.get("environment")).toBe("production");
    const env = screen.getByRole("radiogroup", { name: "Окружение" });
    expect(within(env).getByRole("radio", { name: "production" })).toBeChecked();

    await user.click(within(env).getByRole("radio", { name: "development" }));

    expect(onUrlUpdate.mock.lastCall?.[0].searchParams.get("env")).toBe("development");
    await waitFor(() => expect(captured.portfolio.at(-1)?.get("environment")).toBe("development"));
  });

  it("steps the date but not past yesterday in the owner timezone", async () => {
    vi.useFakeTimers({ toFake: ["Date"], shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-10-13T09:00:00Z"));
    const captured = mockApi([notes]);
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const user = userEvent.setup();
    renderView("", onUrlUpdate);

    await table();
    expect(captured.portfolio[0]?.has("date")).toBe(false);
    expect(
      screen.getByRole("button", { name: /^Выбрать дату: 12\sокт\s2026$/ }),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Следующий день" })).toBeDisabled(),
    );

    await user.click(screen.getByRole("button", { name: "Предыдущий день" }));

    expect(onUrlUpdate.mock.lastCall?.[0].searchParams.get("date")).toBe("2026-10-11");
    await waitFor(() => expect(captured.portfolio.at(-1)?.get("date")).toBe("2026-10-11"));
    expect(
      await screen.findByRole("table", {
        name: /^Портфель приложений за 11\sокт\s2026$/,
      }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Следующий день" })).toBeEnabled();
  });

  it("ignores a malformed date in the URL", async () => {
    const captured = mockApi([notes]);
    renderView("?date=2026-13-45");

    await table();
    expect(captured.portfolio[0]?.has("date")).toBe(false);
  });

  it("lists archived apps on demand", async () => {
    const archived = makeApp({
      id: "0192aaaa-0000-7000-8000-000000000009",
      name: "Old Bot",
      platform: "bot",
      platforms: ["bot"],
      status: "archived",
      archived_at: "2026-09-20T10:00:00Z",
    });
    mockApi([notes], [makeApp({ ...notes.app }), archived]);
    const user = userEvent.setup();
    renderView();

    await table();
    expect(screen.queryByRole("table", { name: "Архив" })).not.toBeInTheDocument();

    await user.click(await screen.findByRole("switch", { name: "Показать архивные (1)" }));

    const archive = screen.getByRole("table", { name: "Архив" });
    expect(within(archive).getByText("Old Bot")).toBeInTheDocument();
    expect(within(archive).getByRole("columnheader", { name: "Платформы" })).toBeInTheDocument();
    expect(within(archive).getByText("Бот")).toBeInTheDocument();
    expect(within(archive).getByText("20 сен 2026")).toBeInTheDocument();
    expect(within(archive).getByRole("link", { name: "Открыть" })).toHaveAttribute(
      "href",
      `/apps/${archived.id}`,
    );
  });

  it("names every platform of an archived app", async () => {
    const archived = makeApp({
      id: "0192aaaa-0000-7000-8000-000000000010",
      name: "Old Game",
      platform: "ios",
      platforms: ["ios", "android"],
      status: "archived",
      archived_at: "2026-09-20T10:00:00Z",
    });
    mockApi([notes], [makeApp({ ...notes.app }), archived]);
    const user = userEvent.setup();
    renderView();

    await table();
    await user.click(await screen.findByRole("switch", { name: "Показать архивные (1)" }));

    const archive = screen.getByRole("table", { name: "Архив" });
    expect(within(archive).getByText("iOS, Android")).toBeInTheDocument();
    expect(within(archive).getByTitle("iOS, Android")).toBeInTheDocument();
  });

  it("shows platform icons by the name and platform shares", async () => {
    const multi = portfolioRow({
      app: {
        id: "0192aaaa-0000-7000-8000-000000000004",
        name: "Notes",
        platform: "ios",
        platforms: ["ios", "android", "web", "macos"],
        status: "active",
      },
      active_date: 100,
      platforms: [
        { platform: "ios", active: 60 },
        { platform: "android", active: 40 },
      ],
    });
    const quiet = portfolioRow({
      app: { ...site.app, id: "0192aaaa-0000-7000-8000-000000000005", name: "Quiet" },
      active_date: 0,
      platforms: [],
    });
    mockApi([multi, quiet]);
    renderView();

    const t = await table();
    const row = rowOf(t, "Notes");
    expect(within(row).getByRole("link", { name: "Notes" })).toBeInTheDocument();
    expect(within(row).getByTitle("iOS, Android, Веб, macOS")).toBeInTheDocument();
    expect(within(row).getByText("+1")).toBeInTheDocument();
    expect(
      within(row).getByRole("img", { name: "iOS 60\u00a0%, Android 40\u00a0%" }),
    ).toBeInTheDocument();
    const quietCells = within(rowOf(t, "Quiet")).getAllByRole("cell");
    expect(quietCells[1]).toHaveTextContent("—");
  });

  it("hides the archive switch when nothing is archived", async () => {
    mockApi([notes]);
    renderView();

    await table();
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });

  it("passes the hidden installs unit and marks it with a badge", async () => {
    const captured = mockApi([notes]);
    renderView("?unit=installs");

    await table();
    expect(captured.portfolio[0]?.get("unit")).toBe("installs");
    expect(screen.getByText("Установки")).toBeInTheDocument();
  });
});

describe("PortfolioTable", () => {
  it("starts a new numeric column descending and the name column ascending", async () => {
    const onSortChange = vi.fn<(sort: PortfolioSort) => void>();
    const user = userEvent.setup();
    renderWithClient(
      <PortfolioTable
        rows={[notes, maps]}
        date="2026-10-12"
        sort={{ key: "active_date", desc: true }}
        onSortChange={onSortChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: /^Новые/ }));
    expect(onSortChange).toHaveBeenLastCalledWith({ key: "new_date", desc: true });
    await user.click(screen.getByRole("button", { name: /^Приложение/ }));
    expect(onSortChange).toHaveBeenLastCalledWith({ key: "name", desc: false });
    await user.click(screen.getByRole("button", { name: /^DAU/ }));
    expect(onSortChange).toHaveBeenLastCalledWith({ key: "active_date", desc: false });
  });
});
