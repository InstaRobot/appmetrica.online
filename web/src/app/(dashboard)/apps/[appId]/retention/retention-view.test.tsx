import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { type OnUrlUpdateFunction, withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Breakdown, Platform, RetentionMatrix } from "@/lib/api/gen/model";
import { errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { RetentionView } from "./retention-view";

const APP_ID = "0192aaaa-0000-7000-8000-000000000001";

vi.mock("next/navigation", () => ({
  useParams: () => ({ appId: APP_ID }),
  usePathname: () => `/apps/${APP_ID}/retention`,
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

// Матрица по параметрам запроса: periods+1 значений, по когорте на каждый день или неделю.
function matrix(params: URLSearchParams, size = 1310): RetentionMatrix {
  const granularity = params.get("cohort_granularity") === "week" ? "week" : "day";
  const periods = Number(params.get("periods") ?? 30);
  const values = Array.from({ length: periods + 1 }, (_, n) => (n === 0 ? 1 : 0.4 / n));
  return {
    cohort_granularity: granularity,
    periods,
    average: size === 0 ? values.map(() => null) : values,
    cohorts: [{ cohort_start: granularity === "week" ? "2026-10-05" : "2026-10-12", size, values }],
  };
}

const breakdown = (params: URLSearchParams): Breakdown => ({
  dimension: (params.get("dimension") ?? "platform") as Breakdown["dimension"],
  metric: "active",
  total: 100,
  rows:
    params.get("dimension") === "platform"
      ? [
          { value: "ios", count: 70, share: 0.7 },
          { value: "android", count: 30, share: 0.3 },
        ]
      : [{ value: "2.1.0", count: 100, share: 1 }],
});

type MockOptions = {
  platforms?: Platform[];
  size?: number;
  error?: { status: number; code: string };
};

function mockApi(opts: MockOptions = {}) {
  const requests: { retention: URLSearchParams[]; breakdown: URLSearchParams[] } = {
    retention: [],
    breakdown: [],
  };
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(
        makeApp({ id: APP_ID, timezone: "Europe/Moscow", platforms: opts.platforms ?? ["ios"] }),
      ),
    ),
    http.get(`*/v1/apps/${APP_ID}/retention`, ({ request }) => {
      const params = new URL(request.url).searchParams;
      requests.retention.push(params);
      if (opts.error) {
        return HttpResponse.json(errorBody(opts.error.code), { status: opts.error.status });
      }
      return HttpResponse.json(matrix(params, opts.size));
    }),
    http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, ({ request }) => {
      const params = new URL(request.url).searchParams;
      requests.breakdown.push(params);
      return HttpResponse.json(breakdown(params));
    }),
  );
  return requests;
}

function renderView(searchParams = "", onUrlUpdate?: OnUrlUpdateFunction) {
  return renderWithClient(<RetentionView />, {
    wrapper: withNuqsTestingAdapter({
      searchParams,
      hasMemory: true,
      ...(onUrlUpdate ? { onUrlUpdate } : {}),
    }),
  });
}

const dayTable = () =>
  screen.findByRole("table", { name: "Retention по дням: доля когорты, вернувшейся в день N" });
const weekTable = () =>
  screen.findByRole("table", {
    name: "Retention по неделям: доля когорты, вернувшейся в неделю N",
  });

beforeEach(() => {
  // 2026-10-13 01:30 по Москве.
  vi.useFakeTimers({ now: new Date("2026-10-12T22:30:00Z"), shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
});

describe("RetentionView", () => {
  it("loads day cohorts for the default period with filters from the URL", async () => {
    const requests = mockApi();
    renderView("?platform=ios&unit=installs");

    const table = await dayTable();
    expect(within(table).getByRole("columnheader", { name: "D30" })).toBeInTheDocument();
    expect(
      await screen.findByRole("img", { name: /^Средняя кривая удержания по дням: D1 40\s%/ }),
    ).toBeInTheDocument();

    const params = requests.retention[0];
    expect(params?.get("cohort_granularity")).toBe("day");
    expect(params?.get("periods")).toBe("30");
    expect(params?.get("from")).toBe("2026-09-13");
    expect(params?.get("to")).toBe("2026-10-12");
    expect(params?.getAll("platform")).toEqual(["ios"]);
    expect(params?.get("environment")).toBe("production");
    expect(params?.get("unit")).toBe("installs");
    expect(
      within(await screen.findByRole("radiogroup", { name: "Платформа" })).getByRole("radio", {
        name: "iOS",
      }),
    ).toHaveAttribute("aria-checked", "true");
    expect(screen.queryByText("Платформа: ios")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /^Выбрать даты: 13\sсен – 12\sокт\s2026$/ }),
    ).toBeVisible();
  });

  it("switches cohort granularity to weeks in the URL", async () => {
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const requests = mockApi();
    const user = userEvent.setup();
    renderView("", onUrlUpdate);
    await dayTable();

    const cohorts = screen.getByRole("radiogroup", { name: "Когорты" });
    expect(within(cohorts).getByRole("radio", { name: "по дням" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await user.click(within(cohorts).getByRole("radio", { name: "по неделям" }));

    expect(onUrlUpdate.mock.lastCall?.[0].queryString).toBe("?cohort=week");
    const table = await weekTable();
    const headers = within(table)
      .getAllByRole("columnheader")
      .map((h) => h.textContent);
    expect(headers.slice(2)).toEqual(Array.from({ length: 13 }, (_, n) => `W${n}`));
    await waitFor(() => expect(requests.retention.at(-1)?.get("cohort_granularity")).toBe("week"));
    const last = requests.retention.at(-1);
    expect(last?.get("periods")).toBe("12");
    expect(last?.get("from")).toBe("2026-07-20");
    expect(last?.get("to")).toBe("2026-10-11");
  });

  it("applies a period preset and changes environment", async () => {
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const requests = mockApi();
    const user = userEvent.setup();
    renderView("", onUrlUpdate);
    await dayTable();

    expect(screen.queryByRole("radio", { name: "Сегодня" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: "90 дней" }));
    expect(onUrlUpdate.mock.lastCall?.[0].queryString).toBe("?period=90d");
    await waitFor(() => expect(requests.retention.at(-1)?.get("from")).toBe("2026-07-16"));
    expect(requests.retention.at(-1)?.get("to")).toBe("2026-10-13");

    await user.click(screen.getByRole("radio", { name: "development" }));
    await waitFor(() => expect(requests.retention.at(-1)?.get("environment")).toBe("development"));
  });

  it("loads filter values only when the filter list opens", async () => {
    const requests = mockApi();
    const user = userEvent.setup();
    renderView("?country=RU");
    await dayTable();
    // Сразу — только наблюдаемые платформы для переключателя, без фильтров отчёта.
    await waitFor(() => expect(requests.breakdown).toHaveLength(1));
    const platform = requests.breakdown[0];
    expect(platform?.get("dimension")).toBe("platform");
    expect(platform?.get("from")).toBe("2026-09-13");
    expect(platform?.get("to")).toBe("2026-10-12");
    expect(platform?.has("country")).toBe(false);

    await user.click(screen.getByRole("button", { name: "Фильтр" }));
    await waitFor(() => expect(requests.breakdown).toHaveLength(4));
    expect(requests.breakdown.slice(1).map((p) => p.get("dimension"))).toEqual([
      "app_version",
      "os_name",
      "country",
    ]);
    expect(requests.breakdown[1]?.getAll("country")).toEqual(["RU"]);
  });

  it("narrows cohorts to the platform from the switcher", async () => {
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const requests = mockApi();
    const user = userEvent.setup();
    renderView("?cohort=week", onUrlUpdate);
    await weekTable();

    // Явная платформа iOS и наблюдаемый Android.
    const switcher = await screen.findByRole("radiogroup", { name: "Платформа" });
    await user.click(await within(switcher).findByRole("radio", { name: "Android" }));
    expect(onUrlUpdate.mock.lastCall?.[0].queryString).toBe("?cohort=week&platform=android");
    await waitFor(() => expect(requests.retention.at(-1)?.getAll("platform")).toEqual(["android"]));

    await user.click(within(switcher).getByRole("radio", { name: "Все" }));
    expect(onUrlUpdate.mock.lastCall?.[0].queryString).toBe("?cohort=week");
  });

  it("shows the empty state when every cohort is empty", async () => {
    mockApi({ size: 0 });
    renderView();

    expect(
      await screen.findByText("Нет новых пользователей в выбранных когортах"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Retention считается по людям, чья первая сессия пришлась на период."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("shows the error in the cohort card", async () => {
    mockApi({ error: { status: 500, code: "internal" } });
    renderView();

    expect(await screen.findByRole("button", { name: "Повторить" })).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.getByRole("radiogroup", { name: "Когорты" })).toBeInTheDocument();
  });

  it("shows range_too_large once under the period picker", async () => {
    mockApi({ error: { status: 400, code: "range_too_large" } });
    renderView("?from=2025-01-01&to=2026-10-12");

    expect(await screen.findByRole("alert")).toHaveTextContent(/период/i);
    expect(screen.queryByRole("button", { name: "Повторить" })).not.toBeInTheDocument();
  });
});
