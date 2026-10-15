import { QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Settings, TelegramTestResult } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { errorBody, settings as makeSettings } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { renderWithLocale } from "@/test/rtl";
import { TelegramTab } from "./telegram-tab";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

function mockSettings(initial: Settings, test?: () => Response) {
  let current = initial;
  const patches: unknown[] = [];
  server.use(
    http.get("*/admin/v1/settings", () => HttpResponse.json(current)),
    http.patch("*/admin/v1/settings", async ({ request }) => {
      const body = (await request.json()) as Partial<Settings>;
      patches.push(body);
      current = { ...current, ...body, updated_at: "2026-10-14T10:00:00Z" };
      return HttpResponse.json(current);
    }),
    http.post(
      "*/admin/v1/settings/telegram/test",
      test ??
        (() =>
          HttpResponse.json({ sent: true, telegram_message_id: 7 } satisfies TelegramTestResult)),
    ),
  );
  return patches;
}

describe("TelegramTab", () => {
  it("fills the form from settings", async () => {
    mockSettings(makeSettings());
    renderWithClient(<TelegramTab />);

    expect(await screen.findByRole("textbox", { name: "Chat ID" })).toHaveValue("123456789");
    expect(screen.getByRole("switch", { name: "Ежедневный дайджест" })).toBeChecked();
    expect(screen.getByLabelText("Время дайджеста")).toHaveValue("09:00");
    expect(screen.getByRole("switch", { name: "Системные алерты" })).toBeChecked();
    expect(screen.getByRole("combobox", { name: /Часовой пояс владельца/ })).toHaveTextContent(
      "Europe/Moscow",
    );
    expect(screen.getByText(/TELEGRAM_BOT_TOKEN/)).toBeInTheDocument();
  });

  it("saves changed settings", async () => {
    const patches = mockSettings(makeSettings());
    renderWithClient(<TelegramTab />);

    const user = userEvent.setup();
    const chat = await screen.findByRole("textbox", { name: "Chat ID" });
    await user.clear(chat);
    await user.type(chat, "-100200300");
    await user.click(screen.getByRole("switch", { name: "Системные алерты" }));
    await user.click(screen.getByRole("button", { name: "Сохранить" }));

    await waitFor(() =>
      expect(patches).toEqual([{ telegram_chat_id: "-100200300", alerts_enabled: false }]),
    );
    expect(toasts.success).toHaveBeenCalledWith("Настройки сохранены");
  });

  it("clears the chat and turns the digest off together", async () => {
    const patches = mockSettings(makeSettings());
    renderWithClient(<TelegramTab />);

    const user = userEvent.setup();
    await user.clear(await screen.findByRole("textbox", { name: "Chat ID" }));
    expect(screen.getByRole("switch", { name: "Ежедневный дайджест" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Сохранить" }));

    await waitFor(() =>
      expect(patches).toEqual([{ telegram_chat_id: null, digest_enabled: false }]),
    );
  });

  it("validates chat id and digest time", async () => {
    const patches = mockSettings(makeSettings());
    renderWithClient(<TelegramTab />);

    const user = userEvent.setup();
    const chat = await screen.findByRole("textbox", { name: "Chat ID" });
    await user.clear(chat);
    await user.type(chat, "@channel");
    const time = screen.getByLabelText("Время дайджеста");
    await user.clear(time);
    await user.type(time, "09:07");
    await user.click(screen.getByRole("button", { name: "Сохранить" }));

    expect(await screen.findByText("Chat ID — число (для групп с минусом)")).toBeInTheDocument();
    expect(screen.getByText("Время с шагом 15 минут")).toBeInTheDocument();
    expect(chat).toHaveAttribute("aria-invalid", "true");
    expect(patches).toEqual([]);
  });

  it("toasts when telegram accepted the test message", async () => {
    mockSettings(makeSettings());
    renderWithClient(<TelegramTab />);

    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Отправить тестовое сообщение" }));
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Сообщение отправлено"));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("shows the telegram error inline without a success toast", async () => {
    mockSettings(makeSettings(), () =>
      HttpResponse.json({ sent: false, error: "chat not found" } satisfies TelegramTestResult),
    );
    renderWithClient(<TelegramTab />);

    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Отправить тестовое сообщение" }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Telegram не принял сообщение: chat not found");
    expect(alert).toHaveTextContent("проверьте Chat ID и что бот добавлен в чат");
    expect(toasts.success).not.toHaveBeenCalled();
  });

  it("disables the test without a saved chat id", async () => {
    mockSettings(makeSettings({ telegram_chat_id: null, digest_enabled: false }));
    renderWithClient(<TelegramTab />);

    expect(
      await screen.findByRole("button", { name: "Отправить тестовое сообщение" }),
    ).toBeDisabled();
  });

  it("toasts when the test request itself fails", async () => {
    mockSettings(makeSettings(), () =>
      HttpResponse.json(errorBody("internal_error"), { status: 500 }),
    );
    renderWithClient(<TelegramTab />);

    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Отправить тестовое сообщение" }));
    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith("Ошибка сервера. Повторите через минуту."),
    );
  });

  it("maps server field errors onto fields", async () => {
    mockSettings(makeSettings());
    server.use(
      http.patch("*/admin/v1/settings", () =>
        HttpResponse.json(
          {
            error: {
              code: "invalid_request",
              message: "invalid",
              request_id: "01J",
              details: { field: "owner_timezone", fields: { owner_timezone: "Неизвестный пояс" } },
            },
          },
          { status: 400 },
        ),
      ),
    );
    renderWithClient(<TelegramTab />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("switch", { name: "Системные алерты" }));
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Неизвестный пояс")).toBeInTheDocument();
  });
});

function renderEnglish(ui: ReactElement) {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return renderWithLocale(<QueryClientProvider client={client}>{ui}</QueryClientProvider>, "en");
}

describe("TelegramTab in English", () => {
  it("labels the form, validates and saves in English", async () => {
    const patches = mockSettings(makeSettings());
    renderEnglish(<TelegramTab />);
    const user = userEvent.setup();

    expect(await screen.findByRole("heading", { name: "Telegram" })).toBeInTheDocument();
    const chat = await screen.findByRole("textbox", { name: "Chat ID" });
    expect(screen.getByText(/to the bot and look up your chat_id/)).toHaveTextContent(
      "TELEGRAM_BOT_TOKEN",
    );
    expect(screen.getByRole("combobox", { name: /Owner time zone/ })).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "Daily digest" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "System alerts" })).toBeChecked();

    await user.clear(chat);
    await user.type(chat, "@channel");
    const time = screen.getByLabelText("Digest time");
    await user.clear(time);
    await user.type(time, "09:07");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(
      await screen.findByText("Chat ID must be a number (negative for groups)"),
    ).toBeInTheDocument();
    expect(screen.getByText("Use 15-minute steps")).toBeInTheDocument();
    expect(patches).toEqual([]);

    await user.clear(chat);
    await user.type(chat, "-100200300");
    await user.clear(time);
    await user.type(time, "09:15");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Settings saved"));
  });

  it("explains a rejected test message in English", async () => {
    mockSettings(makeSettings(), () =>
      HttpResponse.json({ sent: false, error: "chat not found" } satisfies TelegramTestResult),
    );
    renderEnglish(<TelegramTab />);

    await userEvent.setup().click(await screen.findByRole("button", { name: "Send test message" }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Telegram didn't accept the message: chat not found");
    expect(alert).toHaveTextContent("check the Chat ID");
  });

  it("toasts an accepted test message in English", async () => {
    mockSettings(makeSettings());
    renderEnglish(<TelegramTab />);

    await userEvent.setup().click(await screen.findByRole("button", { name: "Send test message" }));
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Message sent"));
  });
});
