import { QueryClientProvider } from "@tanstack/react-query";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RetentionView } from "@/app/(dashboard)/apps/[appId]/retention/retention-view";
import type { RetentionMatrix } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithLocale } from "@/test/rtl";

// Экран S-06 на английском: подписи из словаря en, проценты и даты в формате en-US.
const APP_ID = "0192aaaa-0000-7000-8000-000000000001";

vi.mock("next/navigation", () => ({
  useParams: () => ({ appId: APP_ID }),
  usePathname: () => `/apps/${APP_ID}/retention`,
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

function matrix(params: URLSearchParams, size: number): RetentionMatrix {
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

function mockApi(size = 1310) {
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(makeApp({ id: APP_ID, timezone: "Europe/Moscow", platforms: ["ios"] })),
    ),
    http.get(`*/v1/apps/${APP_ID}/retention`, ({ request }) =>
      HttpResponse.json(matrix(new URL(request.url).searchParams, size)),
    ),
    http.get(`*/v1/apps/${APP_ID}/audience/breakdown`, () =>
      HttpResponse.json({ dimension: "platform", metric: "active", total: 0, rows: [] }),
    ),
  );
}

function renderView(searchParams = "") {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return renderWithLocale(
    <QueryClientProvider client={client}>
      <RetentionView />
    </QueryClientProvider>,
    "en",
    { wrapper: withNuqsTestingAdapter({ searchParams, hasMemory: true }) },
  );
}

beforeEach(() => {
  // 2026-10-13 01:30 по Москве.
  vi.useFakeTimers({ now: new Date("2026-10-12T22:30:00Z"), shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
});

describe("RetentionView in English", () => {
  it("labels the cohorts, the curve and the period in English", async () => {
    mockApi();
    const user = userEvent.setup();
    renderView();

    expect(
      await screen.findByRole("table", {
        name: "Retention by day: share of the cohort that returned on day N",
      }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Average retention curve" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Cohorts" })).toBeInTheDocument();
    expect(
      await screen.findByRole("img", { name: /^Average retention curve by day: D1 40%/ }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /^Pick dates: Sep\s13 – Oct\s12, 2026$/ }),
    ).toBeVisible();

    const cohorts = screen.getByRole("radiogroup", { name: "Cohorts" });
    expect(
      within(cohorts)
        .getAllByRole("radio")
        .map((r) => r.textContent),
    ).toEqual(["By day", "By week"]);
    await user.click(within(cohorts).getByRole("radio", { name: "By week" }));
    expect(
      await screen.findByRole("table", {
        name: "Retention by week: share of the cohort that returned in week N",
      }),
    ).toBeInTheDocument();
  });

  it("shows the empty state in English", async () => {
    mockApi(0);
    renderView();

    expect(await screen.findByText("No new users in the selected cohorts")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Retention is calculated for people whose first session fell within the period.",
      ),
    ).toBeInTheDocument();
  });
});
