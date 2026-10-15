import {
  browserSupportsWebAuthn,
  type RegistrationResponseJSON,
  startRegistration,
} from "@simplewebauthn/browser";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw";
import { renderWithLocale } from "@/test/rtl";
import { SetupPasskeyForm } from "./setup-passkey-form";

vi.mock("@simplewebauthn/browser", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@simplewebauthn/browser")>();
  return { ...actual, browserSupportsWebAuthn: vi.fn(), startRegistration: vi.fn() };
});

const replace = vi.fn<(href: string) => void>();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace }) }));

const attestation: RegistrationResponseJSON = {
  id: "cred",
  rawId: "cred",
  type: "public-key",
  clientExtensionResults: {},
  response: { clientDataJSON: "e30", attestationObject: "AA", transports: ["internal"] },
};
const creationOptions = {
  challenge: "Y2hhbGxlbmdl",
  rp: { id: "localhost", name: "appmetrica.online" },
  user: { id: "dXNlcg", name: "owner", displayName: "owner" },
  pubKeyCredParams: [{ type: "public-key", alg: -7 }],
};
const CEREMONY = "01928f3e-0000-7000-8000-000000000002";

function apiError(status: number, code: string, details?: object) {
  return HttpResponse.json(
    { error: { code, message: "x", request_id: "01J", details } },
    { status },
  );
}

function mockSetupApi(opts: { begin?: () => Response; finish?: (body: unknown) => Response } = {}) {
  server.use(
    http.post(
      "*/api/admin/v1/auth/setup/tok/begin",
      opts.begin ??
        (() =>
          HttpResponse.json({ ceremony_id: CEREMONY, options: { publicKey: creationOptions } })),
    ),
    http.post("*/api/admin/v1/auth/setup/tok/finish", async ({ request }) => {
      const body = await request.json();
      return opts.finish ? opts.finish(body) : new HttpResponse(null, { status: 204 });
    }),
  );
}

const nameInput = () => screen.getByLabelText(/Название устройства/);
const submit = () => screen.getByRole("button", { name: /passkey/ });

beforeEach(() => {
  replace.mockReset();
  vi.mocked(browserSupportsWebAuthn).mockReset().mockReturnValue(true);
  vi.mocked(startRegistration).mockReset().mockResolvedValue(attestation);
});

describe("SetupPasskeyForm", () => {
  it("prefills the device name", () => {
    render(<SetupPasskeyForm token="tok" defaultName="MacBook" />);

    expect(nameInput()).toHaveValue("MacBook");
    expect(nameInput()).toBeRequired();
    expect(screen.getByRole("button", { name: "Создать passkey" })).toBeEnabled();
  });

  // Телефон: поле и кнопка — цели нажатия не меньше 44 px, кнопка на всю ширину карточки; шрифт поля
  // 16 px, чтобы iOS не увеличивал страницу при фокусе.
  it("makes the name field and the submit button 44 px tall on phones", () => {
    render(<SetupPasskeyForm token="tok" defaultName="iPhone" />);

    // 44 px и 16 px на телефоне даёт само поле ввода (components/ui/input.tsx).
    expect(nameInput()).toHaveClass("max-md:h-11", "max-md:text-[16px]");
    expect(screen.getByRole("button", { name: "Создать passkey" })).toHaveClass(
      "w-full",
      "h-11",
      "md:h-10",
    );
  });

  it("validates the name before calling the API", async () => {
    let begun = false;
    mockSetupApi({
      begin: () => {
        begun = true;
        return HttpResponse.json({
          ceremony_id: CEREMONY,
          options: { publicKey: creationOptions },
        });
      },
    });
    render(<SetupPasskeyForm token="tok" defaultName="MacBook" />);

    await userEvent.clear(nameInput());
    await userEvent.type(nameInput(), "   ");
    await userEvent.click(submit());
    expect(screen.getByText("Введите название устройства")).toBeVisible();
    expect(nameInput()).toHaveAttribute("aria-invalid", "true");
    expect(nameInput()).toHaveAccessibleDescription("Введите название устройства");

    await userEvent.clear(nameInput());
    await userEvent.type(nameInput(), "я".repeat(65));
    await userEvent.click(submit());
    expect(screen.getByText("Не длиннее 64 символов")).toBeVisible();

    expect(begun).toBe(false);
    expect(startRegistration).not.toHaveBeenCalled();
  });

  it("registers the passkey and goes to the portfolio", async () => {
    let finishBody: unknown;
    mockSetupApi({
      finish: (body) => {
        finishBody = body;
        return new HttpResponse(null, { status: 204 });
      },
    });
    render(<SetupPasskeyForm token="tok" defaultName="Устройство" />);

    await userEvent.clear(nameInput());
    await userEvent.type(nameInput(), "  MacBook  ");
    await userEvent.click(submit());

    expect(await screen.findByText("Passkey создан. Переходим в дашборд…")).toBeVisible();
    expect(startRegistration).toHaveBeenCalledExactlyOnceWith({ optionsJSON: creationOptions });
    expect(finishBody).toEqual({ ceremony_id: CEREMONY, credential: attestation, name: "MacBook" });
    await waitFor(() => expect(replace).toHaveBeenCalledExactlyOnceWith("/portfolio"), {
      timeout: 2000,
    });
  });

  it("shows the submitting state", async () => {
    mockSetupApi();
    vi.mocked(startRegistration).mockReturnValue(new Promise(() => {}));
    render(<SetupPasskeyForm token="tok" defaultName="MacBook" />);

    await userEvent.click(submit());

    const busy = await screen.findByRole("button", { name: "Создаём passkey…" });
    expect(busy).toBeDisabled();
  });

  it.each([
    ["410 on begin", { begin: () => apiError(410, "setup_token_expired") }],
    ["404 on begin", { begin: () => apiError(404, "not_found") }],
    ["410 on finish", { finish: () => apiError(410, "setup_token_expired") }],
  ])("shows the expired state on %s", async (_, api) => {
    mockSetupApi(api);
    render(<SetupPasskeyForm token="tok" defaultName="MacBook" />);

    await userEvent.click(submit());

    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("Ссылка устарела или уже использована");
    expect(status).toHaveTextContent("appmetrica admin passkey-link");
    expect(screen.queryByLabelText(/Название устройства/)).not.toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });

  it("keeps the form and shows an error when the authenticator is cancelled", async () => {
    mockSetupApi();
    vi.mocked(startRegistration).mockRejectedValue(
      new DOMException("cancelled", "NotAllowedError"),
    );
    render(<SetupPasskeyForm token="tok" defaultName="MacBook" />);

    await userEvent.click(submit());

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Не удалось создать passkey. Попробуйте ещё раз.",
    );
    expect(nameInput()).toHaveValue("MacBook");
    expect(screen.getByRole("button", { name: "Создать passkey" })).toBeEnabled();
  });

  it("shows the server error of the name field", async () => {
    mockSetupApi({
      finish: () => apiError(400, "invalid_request", { fields: { name: "1..64" } }),
    });
    render(<SetupPasskeyForm token="tok" defaultName="MacBook" />);

    await userEvent.click(submit());

    expect(await screen.findByText("Введите название устройства")).toBeVisible();
    expect(nameInput()).toHaveAttribute("aria-invalid", "true");
  });

  it("shows the network error text", async () => {
    mockSetupApi({ begin: () => HttpResponse.error() });
    render(<SetupPasskeyForm token="tok" defaultName="MacBook" />);

    await userEvent.click(submit());

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Нет соединения с сервером. Проверьте подключение.",
    );
  });

  it("disables the form in a browser without WebAuthn", () => {
    vi.mocked(browserSupportsWebAuthn).mockReturnValue(false);
    render(<SetupPasskeyForm token="tok" defaultName="MacBook" />);

    expect(screen.getByRole("button", { name: "Создать passkey" })).toBeDisabled();
    expect(screen.getByText("Браузер не поддерживает passkey")).toBeVisible();
  });

  it("validates and registers in English", async () => {
    mockSetupApi();
    renderWithLocale(<SetupPasskeyForm token="tok" defaultName="MacBook" />, "en");

    const input = screen.getByLabelText(/Device name/);
    await userEvent.clear(input);
    await userEvent.click(screen.getByRole("button", { name: "Create passkey" }));
    expect(input).toHaveAccessibleDescription("Enter a device name");

    await userEvent.type(input, "я".repeat(65));
    await userEvent.click(screen.getByRole("button", { name: "Create passkey" }));
    expect(screen.getByText("64 characters at most")).toBeVisible();

    await userEvent.clear(input);
    await userEvent.type(input, "MacBook");
    await userEvent.click(screen.getByRole("button", { name: "Create passkey" }));
    expect(await screen.findByText("Passkey created. Opening the dashboard…")).toBeVisible();
  });

  it("explains an expired link in English", async () => {
    mockSetupApi({ begin: () => apiError(410, "setup_token_expired") });
    renderWithLocale(<SetupPasskeyForm token="tok" defaultName="MacBook" />, "en");

    await userEvent.click(screen.getByRole("button", { name: "Create passkey" }));

    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("This link has expired or has already been used");
    expect(status).toHaveTextContent(
      "Get a new one: run appmetrica admin passkey-link on the server.",
    );
  });

  it("shows the English error when the authenticator is cancelled", async () => {
    mockSetupApi();
    vi.mocked(startRegistration).mockRejectedValue(
      new DOMException("cancelled", "NotAllowedError"),
    );
    renderWithLocale(<SetupPasskeyForm token="tok" defaultName="MacBook" />, "en");

    await userEvent.click(screen.getByRole("button", { name: "Create passkey" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Couldn't create the passkey. Try again.",
    );
  });
});
