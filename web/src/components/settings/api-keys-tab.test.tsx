import { QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ApiKey } from "@/lib/api/gen/model";
import { makeQueryClient } from "@/lib/query-client";
import { apiKey, appSummary, errorBody } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { renderWithLocale } from "@/test/rtl";
import { ApiKeysTab } from "./api-keys-tab";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

const NOTES = appSummary({ id: "0192aaaa-0000-7000-8000-000000000046", name: "Notes iOS" });
const READER = apiKey({ id: "0192eeee-0000-7000-8000-000000000001", label: "Telegram-отчёт" });
const SECRET = `sk_${"Z".repeat(40)}`;

function mockKeys(items: ApiKey[]) {
  let current = [...items];
  const calls = { created: [] as unknown[], revoked: [] as string[] };
  server.use(
    http.get("*/v1/apps", () => HttpResponse.json({ items: [NOTES] })),
    http.get("*/admin/v1/api-keys", () => HttpResponse.json({ items: current })),
    http.post("*/admin/v1/api-keys", async ({ request }) => {
      const body = (await request.json()) as Partial<ApiKey>;
      calls.created.push(body);
      const created = apiKey({
        id: "0192eeee-0000-7000-8000-000000000002",
        key_prefix: "sk_ZZZZZZZZ",
        label: body.label ?? "",
        scopes: body.scopes ?? [],
        app_id: body.app_id ?? null,
      });
      current = [created, ...current];
      return HttpResponse.json({ api_key: created, secret: SECRET }, { status: 201 });
    }),
    http.post("*/admin/v1/api-keys/:id/revoke", ({ params }) => {
      calls.revoked.push(params.id as string);
      current = current.map((k) =>
        k.id === params.id ? { ...k, revoked_at: "2026-10-14T10:00:00Z" } : k,
      );
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return calls;
}

describe("ApiKeysTab", () => {
  it("lists keys by prefix with scopes and app", async () => {
    mockKeys([READER, apiKey({ label: "Скрипт", scopes: ["read", "delete"], app_id: NOTES.id })]);
    renderWithClient(<ApiKeysTab timezone="Europe/Moscow" />);

    const table = await screen.findByRole("table", { name: "API-ключи" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("sk_a8Kd2xQ1…");
    expect(rows[0]).toHaveTextContent("Все");
    await waitFor(() => expect(rows[1]).toHaveTextContent("Notes iOS"));
    expect(rows[1]).toHaveTextContent("read, delete");
  });

  it("shows the secret once and only the prefix afterwards", async () => {
    const calls = mockKeys([]);
    renderWithClient(<ApiKeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Создать ключ" }));
    let dialog = screen.getByRole("dialog", { name: "Новый API-ключ" });
    await user.type(within(dialog).getByRole("textbox", { name: /Метка/ }), "Скрипт");
    await user.click(within(dialog).getByRole("button", { name: "Создать" }));

    expect(await within(dialog).findByRole("textbox", { name: "секрет" })).toHaveValue(SECRET);
    expect(dialog).toHaveTextContent("Секрет показывается один раз.");
    expect(calls.created).toEqual([{ label: "Скрипт", scopes: ["read"] }]);
    // Закрыть можно только кнопкой: Esc не закрывает диалог с секретом.
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: "Новый API-ключ" })).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Я сохранил ключ" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    expect(await screen.findByText("sk_ZZZZZZZZ…")).toBeInTheDocument();
    expect(screen.queryByDisplayValue(SECRET)).toBeNull();

    await user.click(screen.getByRole("button", { name: "Создать ключ" }));
    dialog = screen.getByRole("dialog", { name: "Новый API-ключ" });
    expect(within(dialog).queryByRole("textbox", { name: "секрет" })).toBeNull();
    expect(within(dialog).getByRole("textbox", { name: /Метка/ })).toHaveValue("");
  });

  it("creates a key for one app with both scopes", async () => {
    const calls = mockKeys([]);
    renderWithClient(<ApiKeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Создать ключ" }));
    const dialog = screen.getByRole("dialog", { name: "Новый API-ключ" });
    await user.type(within(dialog).getByRole("textbox", { name: /Метка/ }), "GDPR");
    await user.click(within(dialog).getByRole("checkbox", { name: "delete" }));
    await user.click(within(dialog).getByRole("combobox", { name: "Приложение" }));
    await user.click(await screen.findByRole("option", { name: "Notes iOS" }));
    await user.click(within(dialog).getByRole("button", { name: "Создать" }));

    await waitFor(() =>
      expect(calls.created).toEqual([
        { label: "GDPR", scopes: ["read", "delete"], app_id: NOTES.id },
      ]),
    );
  });

  it("validates the label and scopes", async () => {
    const calls = mockKeys([]);
    renderWithClient(<ApiKeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Создать ключ" }));
    const dialog = screen.getByRole("dialog", { name: "Новый API-ключ" });
    await user.click(within(dialog).getByRole("checkbox", { name: "read" }));
    await user.click(within(dialog).getByRole("button", { name: "Создать" }));

    expect(within(dialog).getByText("Введите метку")).toBeInTheDocument();
    expect(within(dialog).getByText("Выберите хотя бы одно право")).toBeInTheDocument();
    expect(calls.created).toEqual([]);
  });

  it("revokes a key", async () => {
    const calls = mockKeys([READER]);
    renderWithClient(<ApiKeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Отозвать ключ sk_a8Kd2xQ1…" }));
    const dialog = screen.getByRole("alertdialog", { name: "Отозвать ключ sk_a8Kd2xQ1…?" });
    await user.click(within(dialog).getByRole("button", { name: "Отозвать" }));

    await waitFor(() => expect(calls.revoked).toEqual([READER.id]));
    expect(await screen.findByText("Отозван")).toBeInTheDocument();
  });

  it("shows the empty state with a create button", async () => {
    mockKeys([]);
    renderWithClient(<ApiKeysTab timezone="Europe/Moscow" />);

    expect(await screen.findByRole("heading", { name: "API-ключей нет" })).toBeInTheDocument();
    expect(
      screen.getByText("Ключ нужен для скриптов и интеграций с Read API."),
    ).toBeInTheDocument();
  });

  it("toasts a failed creation and keeps the form", async () => {
    mockKeys([]);
    server.use(
      http.post("*/admin/v1/api-keys", () =>
        HttpResponse.json(errorBody("internal_error"), { status: 500 }),
      ),
    );
    renderWithClient(<ApiKeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Создать ключ" }));
    const dialog = screen.getByRole("dialog", { name: "Новый API-ключ" });
    await user.type(within(dialog).getByRole("textbox", { name: /Метка/ }), "Скрипт");
    await user.click(within(dialog).getByRole("button", { name: "Создать" }));

    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith("Ошибка сервера. Повторите через минуту."),
    );
    expect(within(dialog).getByRole("textbox", { name: /Метка/ })).toHaveValue("Скрипт");
  });
});

function renderEnglish(ui: ReactElement) {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return renderWithLocale(<QueryClientProvider client={client}>{ui}</QueryClientProvider>, "en");
}

describe("ApiKeysTab in English", () => {
  it("lists keys with English headers and dates", async () => {
    mockKeys([
      READER,
      apiKey({
        label: "Script",
        scopes: ["read", "delete"],
        app_id: NOTES.id,
        revoked_at: "2026-10-12T10:00:00Z",
        last_used_at: "2026-10-12T09:30:00Z",
      }),
    ]);
    renderEnglish(<ApiKeysTab timezone="Europe/Moscow" />);

    const table = await screen.findByRole("table", { name: "API keys" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Key", "Label", "Scopes", "App", "Created", "Last used", "Actions"]);
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("All");
    expect(rows[0]).toHaveTextContent("Oct 1, 2026");
    expect(rows[1]).toHaveTextContent("Revoked");
    expect(rows[1]).toHaveTextContent("Oct 12, 2026, 12:30");
    expect(screen.getByRole("heading", { name: "API keys" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create key" })).toBeInTheDocument();
  });

  it("creates a key and shows the secret in English", async () => {
    mockKeys([]);
    renderEnglish(<ApiKeysTab timezone="Europe/Moscow" />);
    const user = userEvent.setup();

    expect(await screen.findByRole("heading", { name: "No API keys yet" })).toBeInTheDocument();
    expect(
      screen.getByText("You need a key for scripts and integrations with the Read API."),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Create key" }));
    const dialog = screen.getByRole("dialog", { name: "New API key" });
    expect(dialog).toHaveTextContent("Authorization: Bearer <secret>");
    expect(within(dialog).getByRole("group", { name: /Scopes/ })).toBeInTheDocument();
    expect(within(dialog).getByRole("combobox", { name: "App" })).toHaveTextContent("All apps");

    await user.click(within(dialog).getByRole("checkbox", { name: "read" }));
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(within(dialog).getByText("Enter a label")).toBeInTheDocument();
    expect(within(dialog).getByText("Select at least one scope")).toBeInTheDocument();

    const label = within(dialog).getByRole("textbox", { name: /Label/ });
    expect(label).toHaveAttribute("placeholder", "For example, Telegram report");
    await user.type(label, "Script");
    await user.click(within(dialog).getByRole("checkbox", { name: "read" }));
    await user.click(within(dialog).getByRole("button", { name: "Create" }));

    expect(await within(dialog).findByRole("textbox", { name: "secret" })).toHaveValue(SECRET);
    expect(dialog).toHaveTextContent("The secret is shown only once.");
    await user.click(within(dialog).getByRole("button", { name: "I've saved the key" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("revokes a key in English", async () => {
    mockKeys([READER]);
    renderEnglish(<ApiKeysTab timezone="Europe/Moscow" />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Revoke key sk_a8Kd2xQ1…" }));
    const dialog = screen.getByRole("alertdialog", { name: "Revoke key sk_a8Kd2xQ1…?" });
    expect(dialog).toHaveTextContent("Read API requests with this key will start getting 401");
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    expect(await screen.findByText("Revoked")).toBeInTheDocument();
    expect(toasts.success).toHaveBeenCalledWith("Key revoked");
  });
});
