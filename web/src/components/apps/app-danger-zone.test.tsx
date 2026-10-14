import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { appMerge, errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { AppDangerZone } from "./app-danger-zone";

const APP_ID = "0192aaaa-0000-7000-8000-000000000045";
const ACTIVE = makeApp({ id: APP_ID, name: "Notes iOS", status: "active" });
const ARCHIVED = makeApp({
  id: APP_ID,
  name: "Notes iOS",
  status: "archived",
  archived_at: "2026-10-10T10:00:00Z",
});

const nav = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: nav.push, replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
}));
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

beforeEach(() => {
  server.use(http.get(`*/admin/v1/apps/${APP_ID}/merges`, () => HttpResponse.json({ items: [] })));
});

function mockActions(deleteResponse?: () => Response) {
  const calls: string[] = [];
  server.use(
    http.post(`*/admin/v1/apps/${APP_ID}/archive`, () => {
      calls.push("archive");
      return new HttpResponse(null, { status: 204 });
    }),
    http.post(`*/admin/v1/apps/${APP_ID}/unarchive`, () => {
      calls.push("unarchive");
      return new HttpResponse(null, { status: 204 });
    }),
    http.delete(`*/admin/v1/apps/${APP_ID}`, async ({ request }) => {
      calls.push(`delete:${((await request.json()) as { confirm_name: string }).confirm_name}`);
      if (deleteResponse) return deleteResponse();
      return HttpResponse.json(
        {
          id: "0192cccc-0000-7000-8000-000000000001",
          app_id: APP_ID,
          subject_type: "app",
          subject_value: APP_ID,
          status: "pending",
          source: "dashboard",
          requested_at: "2026-10-14T10:00:00Z",
          started_at: null,
          completed_at: null,
          error: null,
        },
        { status: 202 },
      );
    }),
  );
  return calls;
}

describe("AppDangerZone", () => {
  it("archives the app after confirmation", async () => {
    const calls = mockActions();
    renderWithClient(<AppDangerZone app={ACTIVE} />);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Архивировать" }));
    const dialog = screen.getByRole("alertdialog", { name: "Архивировать «Notes iOS»?" });
    expect(dialog).toHaveTextContent("403 app_archived");
    await user.click(within(dialog).getByRole("button", { name: "Архивировать" }));

    await waitFor(() => expect(calls).toEqual(["archive"]));
    expect(toasts.success).toHaveBeenCalledWith("Приложение в архиве");
  });

  it("offers to unarchive an archived app", async () => {
    const calls = mockActions();
    renderWithClient(<AppDangerZone app={ARCHIVED} />);

    expect(screen.queryByRole("button", { name: "Архивировать" })).toBeNull();
    await userEvent.setup().click(screen.getByRole("button", { name: "Вернуть из архива" }));

    await waitFor(() => expect(calls).toEqual(["unarchive"]));
    expect(toasts.success).toHaveBeenCalledWith("Приложение возвращено из архива");
  });

  it("deletes the app after typing its name and goes to the portfolio", async () => {
    const calls = mockActions();
    renderWithClient(<AppDangerZone app={ACTIVE} />);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Удалить навсегда" }));
    const dialog = screen.getByRole("alertdialog", { name: "Удалить «Notes iOS» навсегда?" });
    const confirm = within(dialog).getByRole("button", { name: "Удалить навсегда" });
    expect(confirm).toBeDisabled();
    await user.type(
      within(dialog).getByRole("textbox", { name: "Введите Notes iOS" }),
      "Notes iOS",
    );
    await user.click(confirm);

    await waitFor(() => expect(calls).toEqual(["delete:Notes iOS"]));
    await waitFor(() => expect(nav.push).toHaveBeenCalledWith("/portfolio"));
    expect(toasts.success).toHaveBeenCalledWith("Удаление запущено");
  });

  it("shows a name mismatch from the server in the dialog", async () => {
    mockActions(() => HttpResponse.json(errorBody("confirm_name_mismatch"), { status: 409 }));
    renderWithClient(<AppDangerZone app={ACTIVE} />);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Удалить навсегда" }));
    const dialog = screen.getByRole("alertdialog", { name: "Удалить «Notes iOS» навсегда?" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Введите Notes iOS" }),
      "Notes iOS",
    );
    await user.click(within(dialog).getByRole("button", { name: "Удалить навсегда" }));

    expect(await within(dialog).findByText("Название не совпадает")).toBeInTheDocument();
    expect(nav.push).not.toHaveBeenCalled();
  });

  it("toasts when archiving fails", async () => {
    server.use(
      http.post(`*/admin/v1/apps/${APP_ID}/archive`, () =>
        HttpResponse.json(errorBody("internal_error"), { status: 500 }),
      ),
    );
    renderWithClient(<AppDangerZone app={ACTIVE} />);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Архивировать" }));
    const dialog = screen.getByRole("alertdialog", { name: "Архивировать «Notes iOS»?" });
    await user.click(within(dialog).getByRole("button", { name: "Архивировать" }));

    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith("Ошибка сервера. Повторите через минуту."),
    );
  });

  it("opens the merge dialog", async () => {
    renderWithClient(<AppDangerZone app={ACTIVE} />);

    expect(screen.getByText("Объединить с другим приложением")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Объединить…" }));

    expect(await screen.findByRole("dialog", { name: "Объединить приложения" })).toHaveTextContent(
      "Данные выбранного приложения переедут в «Notes iOS».",
    );
  });

  it("disables dangerous actions while a merge is active", async () => {
    server.use(
      http.get(`*/admin/v1/apps/${APP_ID}/merges`, () =>
        HttpResponse.json({
          items: [appMerge({ target_app_id: APP_ID, status: "failed", step: "copy" })],
        }),
      ),
    );
    renderWithClient(<AppDangerZone app={ACTIVE} />);

    expect(await screen.findByText("Идёт объединение — дождитесь завершения")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Объединить…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Архивировать" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Удалить навсегда" })).toBeDisabled();
  });

  it("does not merge into an archived app", async () => {
    renderWithClient(<AppDangerZone app={ARCHIVED} />);

    expect(screen.getByRole("button", { name: "Объединить…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Вернуть из архива" })).toBeEnabled();
  });
});
