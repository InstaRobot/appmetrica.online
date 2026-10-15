import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { type OnUrlUpdateFunction, withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ProfileSummary } from "@/lib/api/gen/model";
import { errorBody, app as makeApp, profileSummary } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { ProfileSearchPanel } from "./profile-search-panel";

const APP_ID = "0192aaaa-0000-7000-8000-000000000023";
const INSTALL = "5d8e2c1a-0b3f-4e5f-9a7b-0c1d2e3f4e5f";

const nav = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: nav.push, replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
}));

function mockSearch(respond: (q: string) => ProfileSummary[] | { status: number }) {
  const queries: string[] = [];
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(makeApp({ id: APP_ID, timezone: "Europe/Moscow" })),
    ),
    http.get(`*/v1/apps/${APP_ID}/profiles`, ({ request }) => {
      const q = new URL(request.url).searchParams.get("q") ?? "";
      queries.push(q);
      const res = respond(q);
      if ("status" in res) {
        return HttpResponse.json(errorBody("internal_error"), { status: res.status });
      }
      return HttpResponse.json({ items: res });
    }),
  );
  return queries;
}

function renderPanel(searchParams = "", onUrlUpdate: OnUrlUpdateFunction = vi.fn()) {
  return renderWithClient(<ProfileSearchPanel appId={APP_ID} />, {
    wrapper: withNuqsTestingAdapter({ searchParams, onUrlUpdate, hasMemory: true }),
  });
}

async function search(value: string) {
  const user = userEvent.setup();
  await user.type(await screen.findByRole("searchbox"), `${value}{Enter}`);
}

beforeEach(() => {
  nav.push.mockClear();
});

describe("ProfileSearchPanel", () => {
  it("opens the profile when exactly one person matches", async () => {
    const queries = mockSearch(() => [profileSummary({ person_key: "u:a/b" })]);
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    renderPanel("", onUrlUpdate);

    await search("42");
    await waitFor(() =>
      expect(nav.push).toHaveBeenCalledWith(`/apps/${APP_ID}/profiles/u%3Aa%2Fb`),
    );
    expect(queries).toEqual(["42"]);
    expect(onUrlUpdate.mock.calls.at(-1)?.[0].searchParams.get("q")).toBe("42");
  });

  it("lists several matches with links to profiles", async () => {
    mockSearch(() => [
      profileSummary(),
      profileSummary({
        person_key: `i:${INSTALL}`,
        user_id: null,
        matched_by: "install_id",
        installs_count: 1,
      }),
    ]);
    renderPanel("?q=42");

    const table = await screen.findByRole("table", { name: "Результаты" });
    const rows = within(table).getAllByRole("row");
    expect(rows).toHaveLength(3);
    expect(rows[1]).toHaveTextContent("u:42");
    expect(rows[1]).toHaveTextContent("3 мар 2026");
    expect(rows[1]).toHaveTextContent("12 окт 2026");
    expect(
      within(rows[2] as HTMLElement).getByRole("link", { name: `Профиль i:${INSTALL}` }),
    ).toHaveAttribute("href", `/apps/${APP_ID}/profiles/i%3A${INSTALL}`);
    expect(screen.getByRole("searchbox")).toHaveValue("42");
    // Поиск из URL (ссылка, «Назад») не уводит на профиль сам.
    expect(nav.push).not.toHaveBeenCalled();
  });

  it("strips the prefix and keeps matches of its type", async () => {
    const queries = mockSearch(() => [
      profileSummary({ person_key: `u:${INSTALL}`, user_id: INSTALL, matched_by: "user_id" }),
      profileSummary({ person_key: "u:7", user_id: "7", matched_by: "install_id" }),
    ]);
    renderPanel();

    await search(`i:${INSTALL}`);
    await waitFor(() => expect(nav.push).toHaveBeenCalledWith(`/apps/${APP_ID}/profiles/u%3A7`));
    expect(queries).toEqual([INSTALL]);
  });

  it("shows an empty state when nothing matches", async () => {
    mockSearch(() => []);
    renderPanel("?q=nobody");

    expect(await screen.findByRole("heading", { name: "Ничего не найдено" })).toBeInTheDocument();
    expect(
      screen.getByText(/только по точному совпадению install_id или user_id/),
    ).toBeInTheDocument();
  });

  it("does not query values longer than user_id allows", async () => {
    const queries = mockSearch(() => [profileSummary()]);
    renderPanel(`?q=u:${"a".repeat(129)}`);

    expect(await screen.findByRole("heading", { name: "Ничего не найдено" })).toBeInTheDocument();
    expect(queries).toEqual([]);
  });

  it("shows only the hint without a query", async () => {
    const queries = mockSearch(() => [profileSummary()]);
    renderPanel();

    expect(await screen.findByText(/Поиск по точному совпадению/)).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(queries).toEqual([]);
  });

  it("shows an error with retry", async () => {
    let n = 0;
    mockSearch(() =>
      n++ === 0 ? { status: 500 } : [profileSummary(), profileSummary({ person_key: "u:43" })],
    );
    renderPanel("?q=42");

    await userEvent.setup().click(await screen.findByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("table", { name: "Результаты" })).toBeInTheDocument();
  });
});

describe("ProfileSearchPanel in English", () => {
  it("lists results with English headers and dates", async () => {
    mockSearch(() => [
      profileSummary({ installs_count: 1234 }),
      profileSummary({ person_key: `i:${INSTALL}`, user_id: null, matched_by: "install_id" }),
    ]);
    renderWithClient(<ProfileSearchPanel appId={APP_ID} />, {
      wrapper: withNuqsTestingAdapter({ searchParams: "?q=42" }),
      locale: "en",
    });

    expect(screen.getByRole("heading", { name: "Profiles" })).toBeInTheDocument();
    const table = await screen.findByRole("table", { name: "Results" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Person", "user_id", "Devices", "First visit", "Last"]);
    const rows = within(table).getAllByRole("row");
    expect(rows[1]).toHaveTextContent("1,234");
    await waitFor(() => expect(rows[1]).toHaveTextContent("Mar 3, 2026"));
    expect(rows[1]).toHaveTextContent("Oct 12, 2026");
    expect(screen.getByRole("link", { name: "Profile u:42" })).toBeInTheDocument();
  });

  it("explains an empty result in English", async () => {
    mockSearch(() => []);
    renderWithClient(<ProfileSearchPanel appId={APP_ID} />, {
      wrapper: withNuqsTestingAdapter({ searchParams: "?q=nobody" }),
      locale: "en",
    });

    expect(await screen.findByRole("heading", { name: "Nothing found" })).toBeInTheDocument();
    expect(screen.getByText(/search only works with an exact install_id or user_id/)).toBeVisible();
  });
});
