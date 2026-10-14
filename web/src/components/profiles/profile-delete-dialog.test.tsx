import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import type { DeletionRequest, Profile } from "@/lib/api/gen/model";
import {
  deletionRequest,
  errorBody,
  app as makeApp,
  profile as makeProfile,
} from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { ProfilePanel } from "./profile-panel";

const APP_ID = "0192aaaa-0000-7000-8000-000000000021";
const INSTALL = "5d8e2c1a-0b3f-4e5f-9a7b-0c1d2e3f4e5f";
const REQUEST_ID = "0192cccc-0000-7000-8000-000000000001";

const nav = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: nav.push, replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
}));
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

type MockOptions = {
  profile?: Profile;
  listed?: DeletionRequest[];
  create?: (body: unknown) => Response;
  poll?: () => DeletionRequest;
};

function mockApi({ profile = makeProfile(), listed = [], create, poll }: MockOptions = {}) {
  const created: unknown[] = [];
  server.use(
    http.get(`*/v1/apps/${APP_ID}`, () =>
      HttpResponse.json(makeApp({ id: APP_ID, name: "Notes iOS", timezone: "Europe/Moscow" })),
    ),
    http.get(`*/v1/apps/${APP_ID}/profiles/:personKey`, () => HttpResponse.json(profile)),
    http.get("*/admin/v1/deletion-requests", ({ request }) => {
      expect(new URL(request.url).searchParams.get("app_id")).toBe(APP_ID);
      return HttpResponse.json({ items: listed });
    }),
    http.post(`*/v1/apps/${APP_ID}/deletion-requests`, async ({ request }) => {
      const body = await request.json();
      created.push(body);
      if (create) return create(body);
      return HttpResponse.json(
        deletionRequest({ id: REQUEST_ID, app_id: APP_ID, ...(body as object) }),
        { status: 202 },
      );
    }),
    http.get(`*/v1/apps/${APP_ID}/deletion-requests/:requestId`, ({ params }) => {
      expect(params.requestId).toBe(REQUEST_ID);
      return HttpResponse.json(poll ? poll() : deletionRequest({ id: REQUEST_ID, app_id: APP_ID }));
    }),
  );
  return created;
}

async function confirmDeletion() {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Удалить данные" }));
  const dialog = screen.getByRole("alertdialog", { name: "Удалить все данные этого человека?" });
  await user.type(within(dialog).getByRole("textbox", { name: "Введите УДАЛИТЬ" }), "УДАЛИТЬ");
  await user.click(within(dialog).getByRole("button", { name: "Удалить данные" }));
  return dialog;
}

describe("ProfileDeleteDialog", () => {
  it("requests deletion for user", async () => {
    const created = mockApi();
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3A42" />);

    await screen.findByRole("heading", { name: "u:42" });
    const dialog = await confirmDeletion();
    expect(dialog).toHaveTextContent("u:42");
    expect(dialog).toHaveTextContent("«Notes iOS»");

    await waitFor(() => expect(created).toEqual([{ subject_type: "user", subject_value: "42" }]));
    const status = await screen.findByRole("group", { name: "Статус удаления" });
    await waitFor(() => expect(status).toHaveTextContent("В очереди"));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Удалить данные" })).toBeDisabled();
    expect(toasts.success).toHaveBeenCalledWith("Запрос на удаление создан");
  });

  it("requests deletion for install", async () => {
    const created = mockApi({
      profile: makeProfile({ person_key: `i:${INSTALL}`, user_id: null }),
    });
    renderWithClient(<ProfilePanel appId={APP_ID} personKey={`i%3A${INSTALL}`} />);

    await screen.findByRole("heading", { name: `i:${INSTALL}` });
    await confirmDeletion();
    await waitFor(() =>
      expect(created).toEqual([{ subject_type: "install", subject_value: INSTALL }]),
    );
  });

  it("keeps the dialog open with the server error", async () => {
    mockApi({
      create: () => HttpResponse.json(errorBody("internal_error"), { status: 500 }),
    });
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3A42" />);

    const dialog = await confirmDeletion();
    const input = within(dialog).getByRole("textbox", { name: "Введите УДАЛИТЬ" });
    await waitFor(() =>
      expect(input).toHaveAccessibleDescription("Ошибка сервера. Повторите через минуту."),
    );
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Статус удаления", hidden: true })).toHaveTextContent(
      "—",
    );
  });

  it("disables deletion while a request is in progress", async () => {
    const running = deletionRequest({ id: REQUEST_ID, app_id: APP_ID, status: "running" });
    mockApi({
      listed: [
        deletionRequest({ app_id: APP_ID, subject_value: "43", status: "pending" }),
        running,
      ],
      poll: () => running,
    });
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3A42" />);

    const status = await screen.findByRole("group", { name: "Статус удаления" });
    await waitFor(() => expect(status).toHaveTextContent("Выполняется"));
    expect(screen.getByRole("button", { name: "Удалить данные" })).toBeDisabled();
  });

  it("shows a finished deletion without redirect", async () => {
    mockApi({
      listed: [
        deletionRequest({
          id: REQUEST_ID,
          app_id: APP_ID,
          status: "done",
          completed_at: "2026-10-12T12:10:00Z",
        }),
      ],
    });
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3A42" />);

    const status = await screen.findByRole("group", { name: "Статус удаления" });
    await waitFor(() => expect(status).toHaveTextContent("Удалено 12 окт 2026 15:10"));
    expect(screen.getByRole("button", { name: "Удалить данные" })).toBeEnabled();
    expect(nav.push).not.toHaveBeenCalled();
  });

  it("returns to search when the deletion is done", async () => {
    nav.push.mockClear();
    toasts.success.mockClear();
    mockApi({
      listed: [deletionRequest({ id: REQUEST_ID, app_id: APP_ID, status: "pending" })],
      poll: () =>
        deletionRequest({
          id: REQUEST_ID,
          app_id: APP_ID,
          status: "done",
          completed_at: "2026-10-12T12:10:00Z",
        }),
    });
    renderWithClient(<ProfilePanel appId={APP_ID} personKey="u%3A42" />);

    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith("Данные удалены", expect.anything()),
    );
    expect(nav.push).not.toHaveBeenCalled();
    await waitFor(() => expect(nav.push).toHaveBeenCalledWith(`/apps/${APP_ID}/profiles`), {
      timeout: 3000,
    });
  });
});
