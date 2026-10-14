import { type RegistrationResponseJSON, startRegistration } from "@simplewebauthn/browser";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import type { Passkey } from "@/lib/api/gen/model";
import { errorBody, passkey } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { PasskeysTab } from "./passkeys-tab";

vi.mock("@simplewebauthn/browser", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@simplewebauthn/browser")>();
  return { ...actual, startRegistration: vi.fn() };
});
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("sonner", () => ({ toast: toasts }));

const MAC = passkey({ id: "0192dddd-0000-7000-8000-000000000001", name: "MacBook Pro" });
const KEY = passkey({
  id: "0192dddd-0000-7000-8000-000000000002",
  name: "YubiKey",
  backup_eligible: false,
  backup_state: false,
  last_used_at: null,
  transports: ["usb"],
});
const CEREMONY = "01928f3e-0000-7000-8000-000000000009";

const attestation: RegistrationResponseJSON = {
  id: "cred",
  rawId: "cred",
  type: "public-key",
  clientExtensionResults: {},
  response: { clientDataJSON: "e30", attestationObject: "AA", transports: ["internal"] },
};

function mockPasskeys(items: Passkey[], opts: { remove?: () => Response } = {}) {
  let current = [...items];
  const calls = { renamed: [] as unknown[], deleted: [] as string[], finished: [] as unknown[] };
  server.use(
    http.get("*/admin/v1/passkeys", () => HttpResponse.json({ items: current })),
    http.patch("*/admin/v1/passkeys/:id", async ({ params, request }) => {
      const body = (await request.json()) as { name: string };
      calls.renamed.push({ id: params.id, ...body });
      current = current.map((p) => (p.id === params.id ? { ...p, name: body.name } : p));
      return HttpResponse.json(current.find((p) => p.id === params.id));
    }),
    http.delete("*/admin/v1/passkeys/:id", ({ params }) => {
      calls.deleted.push(params.id as string);
      if (opts.remove) return opts.remove();
      current = current.filter((p) => p.id !== params.id);
      return new HttpResponse(null, { status: 204 });
    }),
    http.post("*/admin/v1/passkeys/register/begin", () =>
      HttpResponse.json({
        ceremony_id: CEREMONY,
        options: {
          publicKey: {
            challenge: "Y2hhbGxlbmdl",
            rp: { id: "localhost", name: "appmetrica.online" },
            user: { id: "dXNlcg", name: "owner", displayName: "owner" },
            pubKeyCredParams: [{ type: "public-key", alg: -7 }],
          },
        },
      }),
    ),
    http.post("*/admin/v1/passkeys/register/finish", async ({ request }) => {
      const body = (await request.json()) as { name: string };
      calls.finished.push(body);
      const created = passkey({ id: "0192dddd-0000-7000-8000-000000000003", name: body.name });
      current = [...current, created];
      return HttpResponse.json(created, { status: 201 });
    }),
  );
  return calls;
}

describe("PasskeysTab", () => {
  it("lists passkeys with sync status", async () => {
    mockPasskeys([MAC, KEY]);
    renderWithClient(<PasskeysTab timezone="Europe/Moscow" />);

    const table = await screen.findByRole("table", { name: "Passkeys" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("MacBook Pro");
    expect(rows[0]).toHaveTextContent("да");
    expect(rows[1]).toHaveTextContent("YubiKey");
    expect(rows[1]).toHaveTextContent("нет");
    expect(rows[1]).toHaveTextContent("—");
  });

  it("adds a passkey through the browser ceremony", async () => {
    vi.mocked(startRegistration).mockResolvedValue(attestation);
    const calls = mockPasskeys([MAC]);
    renderWithClient(<PasskeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Добавить passkey" }));
    const dialog = screen.getByRole("dialog", { name: "Новый passkey" });
    await user.type(within(dialog).getByRole("textbox", { name: "Название устройства" }), "iPhone");
    await user.click(within(dialog).getByRole("button", { name: "Создать passkey" }));

    await waitFor(() =>
      expect(calls.finished).toEqual([
        { ceremony_id: CEREMONY, credential: attestation, name: "iPhone" },
      ]),
    );
    expect(await screen.findByText("iPhone")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toasts.success).toHaveBeenCalledWith("Passkey добавлен");
  });

  it("requires a device name before the ceremony", async () => {
    const calls = mockPasskeys([MAC]);
    renderWithClient(<PasskeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Добавить passkey" }));
    const dialog = screen.getByRole("dialog", { name: "Новый passkey" });
    await user.clear(within(dialog).getByRole("textbox", { name: "Название устройства" }));
    await user.click(within(dialog).getByRole("button", { name: "Создать passkey" }));

    expect(within(dialog).getByText("Введите название устройства")).toBeInTheDocument();
    expect(startRegistration).not.toHaveBeenCalled();
    expect(calls.finished).toEqual([]);
  });

  it("shows a ceremony failure in the dialog", async () => {
    vi.mocked(startRegistration).mockRejectedValue(new DOMException("no", "NotAllowedError"));
    mockPasskeys([MAC]);
    renderWithClient(<PasskeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Добавить passkey" }));
    const dialog = screen.getByRole("dialog", { name: "Новый passkey" });
    await user.type(within(dialog).getByRole("textbox", { name: "Название устройства" }), "iPhone");
    await user.click(within(dialog).getByRole("button", { name: "Создать passkey" }));

    expect(
      await within(dialog).findByText("Не удалось создать passkey. Попробуйте ещё раз."),
    ).toBeInTheDocument();
  });

  it("renames a passkey", async () => {
    const calls = mockPasskeys([MAC]);
    renderWithClient(<PasskeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Переименовать MacBook Pro" }));
    const dialog = screen.getByRole("dialog", { name: "Переименовать passkey" });
    const input = within(dialog).getByRole("textbox", { name: "Название устройства" });
    expect(input).toHaveValue("MacBook Pro");
    await user.clear(input);
    await user.type(input, "MacBook Air");
    await user.click(within(dialog).getByRole("button", { name: "Сохранить" }));

    await waitFor(() => expect(calls.renamed).toEqual([{ id: MAC.id, name: "MacBook Air" }]));
    expect(await screen.findByText("MacBook Air")).toBeInTheDocument();
  });

  it("explains that the last passkey cannot be deleted", async () => {
    const calls = mockPasskeys([MAC], {
      remove: () => HttpResponse.json(errorBody("last_passkey"), { status: 409 }),
    });
    renderWithClient(<PasskeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Удалить MacBook Pro" }));
    const dialog = screen.getByRole("alertdialog", { name: "Удалить passkey «MacBook Pro»?" });
    await user.click(within(dialog).getByRole("button", { name: "Удалить" }));

    expect(
      await within(dialog).findByText(
        "Нельзя удалить последний passkey — сначала добавьте другой.",
      ),
    ).toBeInTheDocument();
    expect(calls.deleted).toEqual([MAC.id]);
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });

  it("deletes a passkey", async () => {
    mockPasskeys([MAC, KEY]);
    renderWithClient(<PasskeysTab timezone="Europe/Moscow" />);

    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Удалить YubiKey" }));
    const dialog = screen.getByRole("alertdialog", { name: "Удалить passkey «YubiKey»?" });
    await user.click(within(dialog).getByRole("button", { name: "Удалить" }));

    await waitFor(() => expect(screen.queryByText("YubiKey")).toBeNull());
    expect(toasts.success).toHaveBeenCalledWith("Passkey удалён");
  });

  it("shows an error state when the list fails", async () => {
    server.use(
      http.get("*/admin/v1/passkeys", () =>
        HttpResponse.json(errorBody("internal_error"), { status: 500 }),
      ),
    );
    renderWithClient(<PasskeysTab timezone="Europe/Moscow" />);
    expect(await screen.findByRole("button", { name: "Повторить" })).toBeInTheDocument();
  });
});
