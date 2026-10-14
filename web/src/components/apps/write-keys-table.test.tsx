import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import type { WriteKey } from "@/lib/api/gen/model";
import { errorBody, writeKey } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { WriteKeysTable } from "./write-keys-table";

const APP_ID = "0192aaaa-0000-7000-8000-000000000044";
const ACTIVE = writeKey({
  id: "0192bbbb-0000-7000-8000-000000000001",
  app_id: APP_ID,
  key: "pk_7Hq2AAAAAAAAAAAAAAAAAAAAAAAAAAb0",
  label: "iOS 2.x",
});
const REVOKED = writeKey({
  id: "0192bbbb-0000-7000-8000-000000000002",
  app_id: APP_ID,
  key: "pk_Qw9rBBBBBBBBBBBBBBBBBBBBBBBBB1Zx",
  label: "iOS 1.x",
  revoked_at: "2026-10-02T10:00:00Z",
  last_used_at: null,
});

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

function mockKeys(items: WriteKey[] | { status: number }) {
  let current = Array.isArray(items) ? [...items] : [];
  const calls = { created: [] as unknown[], revoked: [] as string[] };
  server.use(
    http.get(`*/admin/v1/apps/${APP_ID}/write-keys`, () => {
      if (!Array.isArray(items)) {
        return HttpResponse.json(errorBody("internal_error"), { status: items.status });
      }
      return HttpResponse.json({ items: current });
    }),
    http.post(`*/admin/v1/apps/${APP_ID}/write-keys`, async ({ request }) => {
      const body = (await request.json()) as { label?: string };
      calls.created.push(body);
      const created = writeKey({
        id: "0192bbbb-0000-7000-8000-000000000003",
        app_id: APP_ID,
        key: "pk_NewKCCCCCCCCCCCCCCCCCCCCCCCCC9zz",
        label: body.label ?? "",
        last_used_at: null,
      });
      current = [created, ...current];
      return HttpResponse.json(created, { status: 201 });
    }),
    http.post(`*/admin/v1/apps/${APP_ID}/write-keys/:keyId/revoke`, ({ params }) => {
      calls.revoked.push(params.keyId as string);
      current = current.map((k) =>
        k.id === params.keyId ? { ...k, revoked_at: "2026-10-14T10:00:00Z" } : k,
      );
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return calls;
}

describe("WriteKeysTable", () => {
  it("lists active and revoked keys", async () => {
    mockKeys([ACTIVE, REVOKED]);
    renderWithClient(<WriteKeysTable appId={APP_ID} timezone="Europe/Moscow" />);

    const table = await screen.findByRole("table", { name: "Write-ключи" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("pk_7Hq2…Ab0");
    expect(rows[0]).toHaveTextContent("iOS 2.x");
    expect(
      within(rows[0] as HTMLElement).getByRole("button", { name: "Отозвать ключ pk_7Hq2…Ab0" }),
    ).toBeEnabled();
    expect(rows[1]).toHaveTextContent("Отозван");
    expect(within(rows[1] as HTMLElement).queryByRole("button", { name: /Отозвать/ })).toBeNull();
  });

  it("creates a key and shows it with a copy button", async () => {
    const calls = mockKeys([ACTIVE]);
    renderWithClient(<WriteKeysTable appId={APP_ID} timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Создать ключ" }));
    const dialog = screen.getByRole("dialog", { name: "Новый write-ключ" });
    await user.type(within(dialog).getByRole("textbox", { name: "Метка" }), "iOS 3.x");
    await user.click(within(dialog).getByRole("button", { name: "Создать" }));

    expect(await within(dialog).findByRole("textbox", { name: "write-ключ" })).toHaveValue(
      "pk_NewKCCCCCCCCCCCCCCCCCCCCCCCCC9zz",
    );
    expect(dialog).toHaveTextContent("Ключ публичный: его можно встраивать в приложение.");
    expect(
      within(dialog).getByRole("button", { name: "Копировать write-ключ" }),
    ).toBeInTheDocument();
    expect(calls.created).toEqual([{ label: "iOS 3.x" }]);

    await user.click(within(dialog).getByRole("button", { name: "Готово" }));
    const table = screen.getByRole("table", { name: "Write-ключи" });
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
  });

  it("limits the label to 64 characters", async () => {
    const calls = mockKeys([]);
    renderWithClient(<WriteKeysTable appId={APP_ID} timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Создать ключ" }));
    const dialog = screen.getByRole("dialog", { name: "Новый write-ключ" });
    await user.type(within(dialog).getByRole("textbox", { name: "Метка" }), "x".repeat(65));
    await user.click(within(dialog).getByRole("button", { name: "Создать" }));

    expect(await within(dialog).findByText("Не длиннее 64 символов")).toBeInTheDocument();
    expect(calls.created).toEqual([]);
  });

  it("revokes a key after confirmation", async () => {
    const calls = mockKeys([ACTIVE]);
    renderWithClient(<WriteKeysTable appId={APP_ID} timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Отозвать ключ pk_7Hq2…Ab0" }));
    const dialog = screen.getByRole("alertdialog", { name: "Отозвать ключ pk_7Hq2…Ab0?" });
    expect(dialog).toHaveTextContent(
      "Через 15 секунд все запросы с этим ключом начнут получать 401",
    );
    await user.click(within(dialog).getByRole("button", { name: "Отозвать" }));

    await waitFor(() => expect(calls.revoked).toEqual([ACTIVE.id]));
    expect(await screen.findByText("Отозван")).toBeInTheDocument();
    expect(toasts.success).toHaveBeenCalledWith("Ключ отозван");
  });

  it("shows an error state with retry when keys fail to load", async () => {
    mockKeys({ status: 500 });
    renderWithClient(<WriteKeysTable appId={APP_ID} timezone="Europe/Moscow" />);

    expect(await screen.findByRole("button", { name: "Повторить" })).toBeInTheDocument();
  });

  it("says when there are no keys", async () => {
    mockKeys([]);
    renderWithClient(<WriteKeysTable appId={APP_ID} timezone="Europe/Moscow" />);

    expect(await screen.findByText("Ключей нет — создайте первый.")).toBeInTheDocument();
  });
});
