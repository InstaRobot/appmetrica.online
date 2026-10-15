import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CreateAppBody } from "@/lib/api/gen/model";
import { errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { mockTimezones } from "@/test/timezones";
import { CreateAppForm } from "./create-app-form";

const nav = vi.hoisted(() => ({ push: vi.fn<(href: string) => void>() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: nav.push, replace: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
}));
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

const created = makeApp({ id: "0192bbbb-0000-7000-8000-000000000001", name: "Notes iOS" });

function mockCreate(respond?: () => Response) {
  const bodies: CreateAppBody[] = [];
  server.use(
    http.post("*/admin/v1/apps", async ({ request }) => {
      bodies.push((await request.json()) as CreateAppBody);
      if (respond) return respond();
      return HttpResponse.json(
        {
          app: created,
          write_keys: [
            {
              id: "0192cccc-0000-7000-8000-000000000001",
              app_id: created.id,
              key: `pk_${"a".repeat(32)}`,
              label: "iOS",
              platform: "ios",
              created_at: "2026-10-13T10:00:00Z",
              revoked_at: null,
              last_used_at: null,
            },
          ],
        },
        { status: 201 },
      );
    }),
  );
  return bodies;
}

async function choosePlatform(user: ReturnType<typeof userEvent.setup>, label: string) {
  await user.click(screen.getByRole("button", { name: label }));
}

beforeEach(() => {
  nav.push.mockReset();
  toasts.success.mockReset();
  toasts.error.mockReset();
});

describe("CreateAppForm", () => {
  beforeEach(mockTimezones);

  it("creates the app and opens its integration screen", async () => {
    const bodies = mockCreate();
    const user = userEvent.setup();
    renderWithClient(<CreateAppForm defaultTimezone="Europe/Moscow" />);

    expect(screen.getByRole("combobox", { name: /Часовой пояс/ })).toHaveTextContent(
      "Europe/Moscow (UTC+3)",
    );
    await user.type(screen.getByRole("textbox", { name: /Название/ }), "  Notes iOS ");
    await choosePlatform(user, "iOS");
    await user.click(screen.getByRole("button", { name: "Создать" }));

    await waitFor(() => expect(nav.push).toHaveBeenCalledWith(`/apps/${created.id}/integration`));
    expect(bodies).toEqual([{ name: "Notes iOS", platforms: ["ios"], timezone: "Europe/Moscow" }]);
    expect(toasts.success).toHaveBeenCalledWith("Приложение создано");
  });

  it("creates an app with several platforms", async () => {
    const bodies = mockCreate();
    const user = userEvent.setup();
    renderWithClient(<CreateAppForm defaultTimezone="Europe/Moscow" />);

    await choosePlatform(user, "iOS");
    await choosePlatform(user, "Android");
    await choosePlatform(user, "Веб");
    await user.type(screen.getByRole("textbox", { name: /Название/ }), "Notes");
    await user.click(screen.getByRole("button", { name: "Создать" }));

    await waitFor(() => expect(nav.push).toHaveBeenCalledWith(`/apps/${created.id}/integration`));
    expect(bodies).toEqual([
      { name: "Notes", platforms: ["ios", "android", "web"], timezone: "Europe/Moscow" },
    ]);
  });

  it("requires a platform", async () => {
    const bodies = mockCreate();
    const user = userEvent.setup();
    renderWithClient(<CreateAppForm defaultTimezone="Europe/Moscow" />);

    await user.type(screen.getByRole("textbox", { name: /Название/ }), "Notes");
    await choosePlatform(user, "Android");
    await choosePlatform(user, "Android");
    await user.click(screen.getByRole("button", { name: "Создать" }));

    const group = screen.getByRole("group", { name: /Платформы/ });
    await waitFor(() =>
      expect(group).toHaveAccessibleDescription("Выберите хотя бы одну платформу"),
    );
    expect(screen.getByRole("button", { name: "iOS" })).toHaveFocus();
    expect(bodies).toEqual([]);

    await choosePlatform(user, "Linux");
    expect(group).toHaveAccessibleDescription(
      "На каждую платформу создадим ключ записи. Платформы можно добавить позже в настройках.",
    );
  });

  it("marks required fields and offers every platform", () => {
    renderWithClient(<CreateAppForm defaultTimezone="Europe/Moscow" />);

    expect(screen.getByRole("textbox", { name: /Название/ })).toHaveAttribute(
      "aria-required",
      "true",
    );
    const group = screen.getByRole("group", { name: /Платформы/ });
    expect(group).toHaveAccessibleDescription(
      "На каждую платформу создадим ключ записи. Платформы можно добавить позже в настройках.",
    );
    const chips = within(group).getAllByRole("button");
    expect(chips.map((chip) => chip.textContent)).toEqual([
      "iOS",
      "Android",
      "Веб",
      "macOS",
      "Windows",
      "Linux",
      "Бот",
      "Бэкенд",
      "Другое",
    ]);
    for (const chip of chips) expect(chip).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("link", { name: "Отмена" })).toHaveAttribute("href", "/portfolio");
  });

  it("shows field errors on submit and focuses the first invalid field", async () => {
    const bodies = mockCreate();
    const user = userEvent.setup();
    renderWithClient(<CreateAppForm defaultTimezone="" />);

    await user.click(screen.getByRole("button", { name: "Создать" }));

    const name = screen.getByRole("textbox", { name: /Название/ });
    expect(name).toHaveAttribute("aria-invalid", "true");
    expect(name).toHaveAccessibleDescription("Введите название");
    expect(name).toHaveFocus();
    expect(screen.getByRole("group", { name: /Платформы/ })).toHaveAccessibleDescription(
      "Выберите хотя бы одну платформу",
    );
    expect(screen.getByRole("combobox", { name: /Часовой пояс/ })).toHaveAccessibleDescription(
      "Выберите часовой пояс",
    );
    expect(bodies).toEqual([]);
  });

  it("limits the name to 64 characters", async () => {
    const user = userEvent.setup();
    renderWithClient(<CreateAppForm defaultTimezone="Europe/Moscow" />);

    const name = screen.getByRole("textbox", { name: /Название/ });
    await user.type(name, "x".repeat(65));
    await user.tab();

    expect(name).toHaveAccessibleDescription("Не длиннее 64 символов");
  });

  it("puts a duplicate name error on the name field", async () => {
    mockCreate(() =>
      HttpResponse.json(errorBody("app_name_taken", "Приложение с таким названием уже есть"), {
        status: 409,
      }),
    );
    const user = userEvent.setup();
    renderWithClient(<CreateAppForm defaultTimezone="Europe/Moscow" />);

    await user.type(screen.getByRole("textbox", { name: /Название/ }), "Notes iOS");
    await choosePlatform(user, "Android");
    await user.click(screen.getByRole("button", { name: "Создать" }));

    const name = screen.getByRole("textbox", { name: /Название/ });
    await waitFor(() =>
      expect(name).toHaveAccessibleDescription("Приложение с таким названием уже есть"),
    );
    expect(name).toHaveFocus();
    expect(nav.push).not.toHaveBeenCalled();
    expect(toasts.error).not.toHaveBeenCalled();
  });

  it("toasts server failures and re-enables the form", async () => {
    mockCreate(() => HttpResponse.json(errorBody("internal_error"), { status: 500 }));
    const user = userEvent.setup();
    renderWithClient(<CreateAppForm defaultTimezone="Europe/Moscow" />);

    await user.type(screen.getByRole("textbox", { name: /Название/ }), "Notes iOS");
    await choosePlatform(user, "iOS");
    await user.click(screen.getByRole("button", { name: "Создать" }));

    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith("Ошибка сервера. Повторите через минуту."),
    );
    expect(screen.getByRole("button", { name: "Создать" })).toBeEnabled();
  });

  it("disables submit while the app is being created", async () => {
    let release = () => {};
    server.use(
      http.post(
        "*/admin/v1/apps",
        () =>
          new Promise<Response>((resolve) => {
            release = () =>
              resolve(HttpResponse.json(errorBody("internal_error"), { status: 500 }));
          }),
      ),
    );
    const user = userEvent.setup();
    renderWithClient(<CreateAppForm defaultTimezone="Europe/Moscow" />);

    await user.type(screen.getByRole("textbox", { name: /Название/ }), "Notes iOS");
    await choosePlatform(user, "Бот");
    await user.click(screen.getByRole("button", { name: "Создать" }));

    const busy = await screen.findByRole("button", { name: "Создаём…" });
    expect(busy).toBeDisabled();
    expect(screen.getByRole("button", { name: "Бот" })).toBeDisabled();
    release();
    await waitFor(() => expect(screen.getByRole("button", { name: "Создать" })).toBeEnabled());
  });

  it("changes the time zone through the picker", async () => {
    const bodies = mockCreate();
    const user = userEvent.setup();
    renderWithClient(<CreateAppForm defaultTimezone="Europe/Moscow" />);

    await user.type(screen.getByRole("textbox", { name: /Название/ }), "Notes iOS");
    await choosePlatform(user, "iOS");
    await user.click(screen.getByRole("combobox", { name: /Часовой пояс/ }));
    const listbox = await screen.findByRole("listbox");
    await user.type(await screen.findByPlaceholderText("Найти часовой пояс…"), "tokyo");
    await user.click(within(listbox).getByRole("option", { name: /Asia\/Tokyo/ }));
    await user.click(screen.getByRole("button", { name: "Создать" }));

    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]?.timezone).toBe("Asia/Tokyo");
    expect(bodies[0]?.platforms).toEqual(["ios"]);
  });
});
