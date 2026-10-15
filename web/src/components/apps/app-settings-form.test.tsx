import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { mockTimezones } from "@/test/timezones";
import { AppSettingsForm, hasClosedDays } from "./app-settings-form";

const APP_ID = "0192aaaa-0000-7000-8000-000000000043";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

// Приложение создано 1 сентября: дни старше D−8 уже закрыты job close_day.
const OLD_APP = makeApp({
  id: APP_ID,
  name: "Notes iOS",
  timezone: "Europe/Moscow",
  created_at: "2026-09-01T10:00:00Z",
  platforms: ["ios", "android"],
  platform: "ios",
});

function mockUpdate(respond?: (body: Record<string, unknown>) => Response) {
  const bodies: Record<string, unknown>[] = [];
  server.use(
    http.patch(`*/admin/v1/apps/${APP_ID}`, async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>;
      bodies.push(body);
      if (respond) return respond(body);
      return HttpResponse.json({ ...OLD_APP, ...body });
    }),
  );
  return bodies;
}

function basics() {
  return within(screen.getByRole("form", { name: "Основное" }));
}

function platforms() {
  return within(screen.getByRole("region", { name: "Платформы" }));
}

async function pickTimezone(name: RegExp) {
  const user = userEvent.setup();
  await user.click(screen.getByRole("combobox", { name: /Часовой пояс/ }));
  await user.type(await screen.findByPlaceholderText("Найти часовой пояс…"), "tokyo");
  await user.click(await screen.findByRole("option", { name }));
}

describe("AppSettingsForm", () => {
  beforeEach(mockTimezones);

  it("stretches the save buttons on phones", async () => {
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    const saves = screen.getAllByRole("button", { name: "Сохранить" });
    expect(saves).toHaveLength(2);
    for (const save of saves) expect(save).toHaveClass("max-md:w-full");
  });

  it("warns about closed days on timezone change", async () => {
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    expect(screen.queryByText(/Закрытые дни не пересчитываются/)).not.toBeInTheDocument();
    await pickTimezone(/Asia\/Tokyo/);

    const warning = screen.getByRole("status", { name: "Смена часового пояса" });
    expect(warning).toHaveTextContent("Закрытые дни не пересчитываются");
    expect(warning).toHaveTextContent("Europe/Moscow");
    expect(screen.getByRole("combobox", { name: /Часовой пояс/ })).toHaveAttribute(
      "aria-describedby",
      expect.stringContaining("timezone-warning"),
    );
  });

  it("does not warn when the app has no closed days yet", async () => {
    const fresh = makeApp({ ...OLD_APP, created_at: new Date().toISOString() });
    renderWithClient(<AppSettingsForm app={fresh} />);

    await pickTimezone(/Asia\/Tokyo/);
    expect(screen.queryByText(/Закрытые дни не пересчитываются/)).not.toBeInTheDocument();
  });

  it("hides the warning when the original timezone is chosen again", async () => {
    renderWithClient(<AppSettingsForm app={OLD_APP} />);
    await pickTimezone(/Asia\/Tokyo/);
    expect(screen.getByText(/Закрытые дни не пересчитываются/)).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole("combobox", { name: /Часовой пояс/ }));
    await user.type(await screen.findByPlaceholderText("Найти часовой пояс…"), "moscow");
    await user.click(await screen.findByRole("option", { name: /Europe\/Moscow/ }));
    expect(screen.queryByText(/Закрытые дни не пересчитываются/)).not.toBeInTheDocument();
  });

  it("keeps save disabled until something changes", async () => {
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    expect(screen.getByRole("textbox", { name: /Название/ })).toHaveValue("Notes iOS");
    expect(screen.queryByText(/изменить нельзя/)).not.toBeInTheDocument();
    const save = basics().getByRole("button", { name: "Сохранить" });
    expect(save).toBeDisabled();

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: /Название/ }), " 2");
    expect(save).toBeEnabled();
    await user.clear(screen.getByRole("textbox", { name: /Название/ }));
    await user.type(screen.getByRole("textbox", { name: /Название/ }), "Notes iOS");
    expect(save).toBeDisabled();
  });

  it("sends only changed fields and toasts on success", async () => {
    const bodies = mockUpdate();
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    const user = userEvent.setup();
    const name = screen.getByRole("textbox", { name: /Название/ });
    await user.clear(name);
    await user.type(name, "  Notes 2  ");
    await user.click(basics().getByRole("button", { name: "Сохранить" }));

    await waitFor(() => expect(bodies).toEqual([{ name: "Notes 2" }]));
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Настройки сохранены"));
  });

  it("saves the new timezone", async () => {
    const bodies = mockUpdate();
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    await pickTimezone(/Asia\/Tokyo/);
    await userEvent.setup().click(basics().getByRole("button", { name: "Сохранить" }));
    await waitFor(() => expect(bodies).toEqual([{ timezone: "Asia/Tokyo" }]));
  });

  it("validates the name before sending", async () => {
    const bodies = mockUpdate();
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    const user = userEvent.setup();
    const name = screen.getByRole("textbox", { name: /Название/ });
    await user.clear(name);
    await user.type(name, "   ");
    await user.click(basics().getByRole("button", { name: "Сохранить" }));

    expect(await screen.findByText("Введите название")).toBeInTheDocument();
    expect(name).toHaveAttribute("aria-invalid", "true");
    expect(name).toHaveFocus();
    expect(bodies).toEqual([]);
  });

  it("puts a duplicate name error on the name field", async () => {
    mockUpdate(() => HttpResponse.json(errorBody("app_name_taken"), { status: 409 }));
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    const user = userEvent.setup();
    const name = screen.getByRole("textbox", { name: /Название/ });
    await user.clear(name);
    await user.type(name, "Weather Android");
    await user.click(basics().getByRole("button", { name: "Сохранить" }));

    const card = screen.getByRole("form", { name: "Основное" });
    expect(
      await within(card).findByText("Приложение с таким названием уже есть"),
    ).toBeInTheDocument();
    expect(toasts.error).not.toHaveBeenCalled();
  });

  it("toasts server failures", async () => {
    mockUpdate(() => HttpResponse.json(errorBody("internal_error"), { status: 500 }));
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: /Название/ }), "!");
    await user.click(basics().getByRole("button", { name: "Сохранить" }));

    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith("Ошибка сервера. Повторите через минуту."),
    );
    expect(basics().getByRole("button", { name: "Сохранить" })).toBeEnabled();
  });
});

describe("AppSettingsForm platforms", () => {
  afterEach(() => {
    // biome-ignore lint/suspicious/noDocumentCookie: тест демо-режима ставит и снимает cookie am_demo.
    document.cookie = "am_demo=; max-age=0; path=/";
    vi.clearAllMocks();
  });

  it("shows the current platforms and keeps save disabled until the set changes", async () => {
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    const section = platforms();
    expect(await section.findByRole("button", { name: "iOS" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(section.getByRole("button", { name: "Android" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(section.getByRole("button", { name: "Веб" })).toHaveAttribute("aria-pressed", "false");
    expect(
      section.getByText("Убрать платформу — не удалить её данные: события и ключи сохраняются."),
    ).toBeInTheDocument();
    const save = section.getByRole("button", { name: "Сохранить" });
    expect(save).toBeDisabled();

    const user = userEvent.setup();
    await user.click(section.getByRole("button", { name: "Веб" }));
    expect(save).toBeEnabled();
    await user.click(section.getByRole("button", { name: "Веб" }));
    expect(save).toBeDisabled();
  });

  it("saves the platforms", async () => {
    const bodies = mockUpdate();
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    const section = platforms();
    const user = userEvent.setup();
    await user.click(await section.findByRole("button", { name: "Android" }));
    await user.click(section.getByRole("button", { name: "Веб" }));
    await user.click(section.getByRole("button", { name: "Сохранить" }));

    await waitFor(() => expect(bodies).toEqual([{ platforms: ["ios", "web"] }]));
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Платформы сохранены"));
  });

  it("follows the platforms set when the app is refetched", async () => {
    const bodies = mockUpdate();
    const { rerender } = renderWithClient(<AppSettingsForm app={OLD_APP} />);
    const user = userEvent.setup();
    await user.click(await platforms().findByRole("button", { name: "Веб" }));
    expect(platforms().getByRole("button", { name: "Сохранить" })).toBeEnabled();

    // Слияние добавило цели платформы источника: карточка показывает набор с сервера, а не черновик.
    rerender(<AppSettingsForm app={{ ...OLD_APP, platforms: ["ios", "android", "linux"] }} />);

    const section = platforms();
    expect(await section.findByRole("button", { name: "Linux" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(section.getByRole("button", { name: "Веб" })).toHaveAttribute("aria-pressed", "false");
    expect(section.getByRole("button", { name: "Сохранить" })).toBeDisabled();
    expect(bodies).toEqual([]);
  });

  it("keeps at least one platform", async () => {
    const bodies = mockUpdate();
    renderWithClient(<AppSettingsForm app={{ ...OLD_APP, platforms: ["ios"] }} />);

    const section = platforms();
    await userEvent.setup().click(await section.findByRole("button", { name: "iOS" }));

    const save = section.getByRole("button", { name: "Сохранить" });
    expect(save).toBeDisabled();
    expect(section.getByText("Оставьте хотя бы одну платформу")).toBeInTheDocument();
    expect(section.getByRole("group", { name: "Платформы" })).toHaveAttribute(
      "aria-describedby",
      expect.stringContaining("platforms-empty"),
    );
    expect(bodies).toEqual([]);
  });

  it("toasts a failed save and lets the user retry", async () => {
    mockUpdate(() => HttpResponse.json(errorBody("internal_error"), { status: 500 }));
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    const section = platforms();
    const user = userEvent.setup();
    await user.click(await section.findByRole("button", { name: "Веб" }));
    await user.click(section.getByRole("button", { name: "Сохранить" }));

    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith("Ошибка сервера. Повторите через минуту."),
    );
    expect(toasts.success).not.toHaveBeenCalledWith("Платформы сохранены");
    expect(section.getByRole("button", { name: "Сохранить" })).toBeEnabled();
  });

  it("disables the platforms in the demo", async () => {
    // biome-ignore lint/suspicious/noDocumentCookie: тест демо-режима ставит и снимает cookie am_demo.
    document.cookie = "am_demo=1; path=/";
    renderWithClient(<AppSettingsForm app={OLD_APP} />);

    const section = platforms();
    expect(await section.findByRole("button", { name: "Веб" })).toBeDisabled();
    expect(section.getByRole("button", { name: "Сохранить" })).toBeDisabled();
  });
});

describe("hasClosedDays", () => {
  const now = new Date("2026-10-14T12:00:00Z");

  it.each([
    ["2026-09-01T10:00:00Z", true],
    ["2026-10-05T12:00:00Z", true],
    ["2026-10-07T12:00:00Z", false],
    ["2026-10-14T11:00:00Z", false],
    ["not a date", false],
  ])("created_at %s → %s", (createdAt, expected) => {
    expect(hasClosedDays(createdAt, now)).toBe(expected);
  });
});

describe("AppSettingsForm in English", () => {
  beforeEach(mockTimezones);

  it("edits the general settings in English", async () => {
    const bodies = mockUpdate();
    renderWithClient(<AppSettingsForm app={OLD_APP} />, { locale: "en" });

    const card = within(screen.getByRole("form", { name: "General" }));
    const name = card.getByRole("textbox", { name: /Name/ });
    expect(card.getByRole("combobox", { name: /Time zone/ })).toHaveAccessibleDescription(
      "Changes day boundaries in reports; closed days aren't recalculated.",
    );
    const user = userEvent.setup();
    await user.clear(name);
    await user.type(name, "   ");
    await user.click(card.getByRole("button", { name: "Save" }));
    expect(name).toHaveAccessibleDescription("Enter a name");

    await user.clear(name);
    await user.type(name, "Notes 2");
    await user.click(card.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodies).toEqual([{ name: "Notes 2" }]));
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Settings saved"));
  });

  it("warns about closed days in English", async () => {
    renderWithClient(<AppSettingsForm app={OLD_APP} />, { locale: "en" });

    const user = userEvent.setup();
    await user.click(screen.getByRole("combobox", { name: /Time zone/ }));
    await user.click(await screen.findByRole("option", { name: /Asia\/Tokyo/ }));
    expect(screen.getByRole("status", { name: "Time zone change" })).toHaveTextContent(
      "Closed days aren't recalculated: days older than 8 days have already been counted in the Europe/Moscow time zone. The new time zone applies to the remaining days.",
    );
  });

  it("edits the platforms in English", async () => {
    const bodies = mockUpdate();
    renderWithClient(<AppSettingsForm app={{ ...OLD_APP, platforms: ["ios"] }} />, {
      locale: "en",
    });

    const section = within(screen.getByRole("region", { name: "Platforms" }));
    expect(
      section.getByText("Removing a platform doesn't delete its data: events and keys are kept."),
    ).toBeInTheDocument();
    const user = userEvent.setup();
    await user.click(await section.findByRole("button", { name: "iOS" }));
    expect(section.getByText("Keep at least one platform")).toBeInTheDocument();

    await user.click(section.getByRole("button", { name: "Android" }));
    await user.click(section.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(bodies).toEqual([{ platforms: ["android"] }]));
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Platforms saved"));
  });
});
