import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import {
  errorBody,
  app as makeApp,
  profile as makeProfile,
  profileInstall,
  profileSession,
} from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { ProfilePanel } from "./profile-panel";

const APP_ID = "0192aaaa-0000-7000-8000-000000000022";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
}));
vi.mock("@/lib/toast", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

type ProfileHandler = Parameters<typeof http.get>[1];

function mockApi(onProfile: ProfileHandler) {
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(makeApp({ id: APP_ID, timezone: "Europe/Moscow" })),
    ),
    http.get(`*/v1/apps/${APP_ID}/profiles/:personKey`, onProfile),
    http.get("*/admin/v1/deletion-requests", () => HttpResponse.json({ items: [] })),
  );
}

describe("ProfilePanel", () => {
  it("shows the profile with installs and sessions in the app timezone", async () => {
    const urls: string[] = [];
    mockApi(({ request }) => {
      urls.push(request.url);
      return HttpResponse.json(
        makeProfile({
          person_key: "u:a/b c",
          user_id: "a/b c",
          installs: [profileInstall(), profileInstall({ install_id: "short", country: "" })],
          sessions: [
            profileSession(),
            profileSession({
              started_at: "2026-10-12T06:15:00Z",
              ended_at: null,
              duration_ms: null,
            }),
          ],
        }),
      );
    });
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3Aa%2Fb%20c" />);

    expect(await screen.findByRole("heading", { name: "u:a/b c" })).toBeInTheDocument();
    expect(new URL(urls[0] ?? "").pathname).toBe(`/api/v1/apps/${APP_ID}/profiles/u%3Aa%2Fb%20c`);
    expect(new URL(urls[0] ?? "").searchParams.get("sessions_limit")).toBe("100");
    expect(screen.getByRole("link", { name: "К поиску" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/profiles`,
    );
    expect(screen.getByRole("button", { name: "Копировать person_key" })).toBeInTheDocument();
    expect(screen.getByText("user_id:").parentElement).toHaveTextContent("a/b c");
    expect(screen.getByText(/Первый визит/).parentElement).toHaveTextContent("3 мар 2026");

    const installs = screen.getByRole("table", { name: "Устройства" });
    const rows = within(installs).getAllByRole("row");
    expect(rows).toHaveLength(3);
    expect(rows[1]).toHaveTextContent("5d8e…4e5f");
    expect(rows[1]).toHaveTextContent("iOS 26.0");
    expect(rows[1]).toHaveTextContent("RU · Москва");
    expect(rows[1]).toHaveTextContent("iPhone17,1");
    expect(rows[2]).toHaveTextContent("—");

    const sessions = screen.getByRole("table", { name: "Сессии" });
    const sessionRows = within(sessions).getAllByRole("row");
    expect(sessionRows[1]).toHaveTextContent("12 окт 2026 14:01");
    expect(sessionRows[1]).toHaveTextContent("4 мин 12 с");
    expect(sessionRows[2]).toHaveTextContent("12 окт 2026 09:15");
    expect(sessionRows[2]).toHaveTextContent("— (нет session_end)");
    expect(screen.queryByRole("button", { name: "Показать ещё" })).not.toBeInTheDocument();
  });

  it("loads more sessions by before cursor", async () => {
    const befores: (string | null)[] = [];
    mockApi(({ request }) => {
      const before = new URL(request.url).searchParams.get("before");
      befores.push(before);
      return HttpResponse.json(
        before
          ? makeProfile({
              sessions: [profileSession({ session_id: "s2", started_at: "2026-10-01T10:00:00Z" })],
              next_before: null,
            })
          : makeProfile({
              sessions: [profileSession({ session_id: "s1", started_at: "2026-10-12T11:01:00Z" })],
              next_before: "2026-10-12T11:01:00Z",
            }),
      );
    });
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3A42" />);

    const more = await screen.findByRole("button", { name: "Показать ещё" });
    await userEvent.setup().click(more);

    const sessions = screen.getByRole("table", { name: "Сессии" });
    await waitFor(() => expect(within(sessions).getAllByRole("row")).toHaveLength(3));
    expect(within(sessions).getAllByRole("row")[2]).toHaveTextContent("1 окт 2026 13:00");
    expect(befores).toEqual([null, "2026-10-12T11:01:00Z"]);
    expect(screen.queryByRole("button", { name: "Показать ещё" })).not.toBeInTheDocument();
  });

  it("shows not found for an unknown profile", async () => {
    mockApi(() => HttpResponse.json(errorBody("profile_not_found"), { status: 404 }));
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3A404" />);

    expect(await screen.findByRole("heading", { name: "Профиль не найден" })).toBeInTheDocument();
    expect(screen.getByText("Возможно, данные удалены или ещё не поступили.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "К поиску" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/profiles`,
    );
  });

  it("shows not found for a malformed person key without a request", async () => {
    const urls: string[] = [];
    mockApi(({ request }) => {
      urls.push(request.url);
      return HttpResponse.json(makeProfile());
    });
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="x%3A1" />);

    expect(await screen.findByRole("heading", { name: "Профиль не найден" })).toBeInTheDocument();
    expect(urls).toEqual([]);
  });

  it("shows an error with retry", async () => {
    let n = 0;
    mockApi(() =>
      n++ === 0
        ? HttpResponse.json(errorBody("internal_error"), { status: 500 })
        : HttpResponse.json(makeProfile()),
    );
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3A42" />);

    await userEvent.setup().click(await screen.findByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("heading", { name: "u:42" })).toBeInTheDocument();
  });

  it("shows a skeleton while loading", () => {
    mockApi(() => HttpResponse.json(makeProfile()));
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3A42" />);
    expect(screen.getAllByRole("status", { name: "Загрузка…" }).length).toBeGreaterThan(0);
  });
});
