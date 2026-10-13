import {
  type AuthenticationResponseJSON,
  browserSupportsWebAuthn,
  browserSupportsWebAuthnAutofill,
  startAuthentication,
  WebAuthnError,
} from "@simplewebauthn/browser";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw";
import { PasskeyButton } from "./passkey-button";

vi.mock("@simplewebauthn/browser", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@simplewebauthn/browser")>();
  return {
    ...actual,
    browserSupportsWebAuthn: vi.fn(),
    browserSupportsWebAuthnAutofill: vi.fn(),
    startAuthentication: vi.fn(),
  };
});

const replace = vi.fn<(href: string) => void>();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace }) }));

const assertion: AuthenticationResponseJSON = {
  id: "cred",
  rawId: "cred",
  type: "public-key",
  clientExtensionResults: {},
  response: {
    clientDataJSON: "e30",
    authenticatorData: "AA",
    signature: "AA",
    userHandle: "dXNlcg",
  },
};
const requestOptions = {
  challenge: "Y2hhbGxlbmdl",
  rpId: "localhost",
  userVerification: "required",
};

let finishCalls = 0;

function mockLoginApi(finish: () => Response = () => new HttpResponse(null, { status: 204 })) {
  server.use(
    http.post("*/api/admin/v1/auth/login/begin", () =>
      HttpResponse.json({
        ceremony_id: "01928f3e-0000-7000-8000-000000000001",
        options: { publicKey: requestOptions, mediation: "conditional" },
      }),
    ),
    http.post("*/api/admin/v1/auth/login/finish", () => {
      finishCalls++;
      return finish();
    }),
  );
}

const button = () => screen.getByRole("button", { name: /passkey|Ожидаем/ });

beforeEach(() => {
  finishCalls = 0;
  replace.mockReset();
  vi.mocked(browserSupportsWebAuthn).mockReset().mockReturnValue(true);
  vi.mocked(browserSupportsWebAuthnAutofill).mockReset().mockResolvedValue(false);
  vi.mocked(startAuthentication).mockReset().mockResolvedValue(assertion);
});

describe("PasskeyButton", () => {
  it("shows unsupported state", () => {
    vi.mocked(browserSupportsWebAuthn).mockReturnValue(false);

    render(<PasskeyButton />);

    expect(screen.getByRole("button", { name: "Войти с passkey" })).toBeDisabled();
    expect(screen.getByText("Браузер не поддерживает passkey")).toBeVisible();
    const help = screen.getByRole("link", { name: /passkey/i });
    expect(help).toHaveAttribute("href", expect.stringMatching(/^https:\/\//));
    expect(help).toHaveAttribute("rel", expect.stringContaining("noreferrer"));
    expect(startAuthentication).not.toHaveBeenCalled();
  });

  it("logs in with passkey and replaces the route with next", async () => {
    mockLoginApi();
    render(<PasskeyButton next="/apps/1?from=2026-10-01" />);

    await userEvent.click(screen.getByRole("button", { name: "Войти с passkey" }));

    await waitFor(() => expect(replace).toHaveBeenCalledExactlyOnceWith("/apps/1?from=2026-10-01"));
    expect(startAuthentication).toHaveBeenCalledExactlyOnceWith({
      optionsJSON: requestOptions,
      useBrowserAutofill: false,
    });
    expect(finishCalls).toBe(1);
  });

  it("ignores an unsafe next and goes to the portfolio", async () => {
    mockLoginApi();
    render(<PasskeyButton next="//evil.example/phish" />);

    await userEvent.click(screen.getByRole("button", { name: "Войти с passkey" }));

    await waitFor(() => expect(replace).toHaveBeenCalledExactlyOnceWith("/"));
  });

  it("disables the button while waiting for the authenticator", async () => {
    mockLoginApi();
    vi.mocked(startAuthentication).mockReturnValue(new Promise(() => {}));
    render(<PasskeyButton />);

    await userEvent.click(screen.getByRole("button", { name: "Войти с passkey" }));

    const waiting = await screen.findByRole("button", { name: "Ожидаем подтверждение…" });
    expect(waiting).toBeDisabled();
  });

  it("shows an error, calls onError and returns focus when the user cancels", async () => {
    mockLoginApi();
    vi.mocked(startAuthentication).mockRejectedValue(
      new DOMException("cancelled", "NotAllowedError"),
    );
    const onError = vi.fn<(message: string) => void>();
    render(<PasskeyButton onError={onError} />);

    await userEvent.click(screen.getByRole("button", { name: "Войти с passkey" }));

    const message =
      "Не удалось войти. Подтверждение отменено или ключ не подошёл. Попробуйте ещё раз.";
    expect(await screen.findByRole("alert")).toHaveTextContent(message);
    expect(onError).toHaveBeenCalledExactlyOnceWith(message);
    expect(button()).toBeEnabled();
    expect(button()).toHaveFocus();
    expect(replace).not.toHaveBeenCalled();
  });

  it("shows the network error text", async () => {
    server.use(http.post("*/api/admin/v1/auth/login/begin", () => HttpResponse.error()));
    render(<PasskeyButton />);

    await userEvent.click(screen.getByRole("button", { name: "Войти с passkey" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Нет соединения с сервером. Проверьте подключение.",
    );
  });

  it("shows the error when the server rejects the passkey", async () => {
    mockLoginApi(() =>
      HttpResponse.json(
        { error: { code: "webauthn_verification_failed", message: "x", request_id: "01J" } },
        { status: 422 },
      ),
    );
    render(<PasskeyButton />);

    await userEvent.click(screen.getByRole("button", { name: "Войти с passkey" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось войти.");
    expect(replace).not.toHaveBeenCalled();
  });

  it("starts conditional UI autofill when the browser supports it", async () => {
    mockLoginApi();
    vi.mocked(browserSupportsWebAuthnAutofill).mockResolvedValue(true);
    const { container } = render(<PasskeyButton next="/settings" />);

    const input = container.querySelector('input[autocomplete="username webauthn"]');
    expect(input).not.toBeNull();
    await waitFor(() =>
      expect(startAuthentication).toHaveBeenCalledWith({
        optionsJSON: requestOptions,
        useBrowserAutofill: true,
      }),
    );
    await waitFor(() => expect(replace).toHaveBeenCalledExactlyOnceWith("/settings"));
  });

  it("keeps quiet when conditional UI is aborted", async () => {
    mockLoginApi();
    vi.mocked(browserSupportsWebAuthnAutofill).mockResolvedValue(true);
    vi.mocked(startAuthentication).mockRejectedValue(
      new WebAuthnError({
        message: "aborted",
        code: "ERROR_CEREMONY_ABORTED",
        cause: new DOMException("aborted", "AbortError"),
      }),
    );
    render(<PasskeyButton />);

    await waitFor(() => expect(startAuthentication).toHaveBeenCalled());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Войти с passkey" })).toBeEnabled();
  });
});
