import {
  type AuthenticationResponseJSON,
  type RegistrationResponseJSON,
  startAuthentication,
  startRegistration,
  WebAuthnError,
} from "@simplewebauthn/browser";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/fetcher";
import { server } from "@/test/msw";
import {
  deviceNameFromUserAgent,
  isCeremonyAborted,
  isSetupLinkExpired,
  loginWithPasskey,
  passkeyErrorMessage,
  registerPasskeyBySetupLink,
  validateDeviceName,
} from "./webauthn";

vi.mock("@simplewebauthn/browser", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@simplewebauthn/browser")>();
  return { ...actual, startAuthentication: vi.fn(), startRegistration: vi.fn() };
});

const requestOptions = {
  challenge: "Y2hhbGxlbmdl",
  rpId: "localhost",
  userVerification: "required",
};
const creationOptions = {
  challenge: "Y2hhbGxlbmdl",
  rp: { id: "localhost", name: "appmetrica.online" },
  user: { id: "dXNlcg", name: "owner", displayName: "owner" },
  pubKeyCredParams: [{ type: "public-key", alg: -7 }],
};
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
const attestation: RegistrationResponseJSON = {
  id: "cred",
  rawId: "cred",
  type: "public-key",
  clientExtensionResults: {},
  response: { clientDataJSON: "e30", attestationObject: "AA", transports: ["internal"] },
};
const CEREMONY = "01928f3e-0000-7000-8000-000000000001";

beforeEach(() => {
  vi.mocked(startAuthentication).mockReset().mockResolvedValue(assertion);
  vi.mocked(startRegistration).mockReset().mockResolvedValue(attestation);
});

describe("deviceNameFromUserAgent", () => {
  it.each([
    [
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/26.0 Safari/605.1.15",
      "MacBook",
    ],
    [
      "Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148",
      "iPhone",
    ],
    [
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/141.0 Safari/537.36",
      "Windows PC",
    ],
    ["Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/141.0 Safari/537.36", "Устройство"],
    ["", "Устройство"],
  ])("%s → %s", (ua, name) => {
    expect(deviceNameFromUserAgent(ua)).toBe(name);
  });
});

describe("validateDeviceName", () => {
  it("accepts 1..64 characters after trim", () => {
    expect(validateDeviceName("  MacBook  ")).toBeNull();
    expect(validateDeviceName("я".repeat(64))).toBeNull();
  });

  it("rejects empty and too long names", () => {
    expect(validateDeviceName("")).toBe("Введите название устройства");
    expect(validateDeviceName("   ")).toBe("Введите название устройства");
    expect(validateDeviceName("я".repeat(65))).toBe("Не длиннее 64 символов");
  });
});

describe("loginWithPasskey", () => {
  it("passes options.publicKey to startAuthentication and finishes with the ceremony id", async () => {
    let finishBody: unknown;
    server.use(
      http.post("*/api/admin/v1/auth/login/begin", () =>
        HttpResponse.json({
          ceremony_id: CEREMONY,
          options: { publicKey: requestOptions, mediation: "conditional" },
        }),
      ),
      http.post("*/api/admin/v1/auth/login/finish", async ({ request }) => {
        finishBody = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );

    await loginWithPasskey({ autofill: true });

    expect(startAuthentication).toHaveBeenCalledExactlyOnceWith({
      optionsJSON: requestOptions,
      useBrowserAutofill: true,
    });
    expect(finishBody).toEqual({ ceremony_id: CEREMONY, credential: assertion });
  });

  it("rejects options without publicKey and does not call the authenticator", async () => {
    server.use(
      http.post("*/api/admin/v1/auth/login/begin", () =>
        HttpResponse.json({ ceremony_id: CEREMONY, options: { mediation: "conditional" } }),
      ),
    );

    await expect(loginWithPasskey({ autofill: false })).rejects.toThrow();
    expect(startAuthentication).not.toHaveBeenCalled();
  });

  it("propagates the API error of finishLogin", async () => {
    server.use(
      http.post("*/api/admin/v1/auth/login/begin", () =>
        HttpResponse.json({ ceremony_id: CEREMONY, options: { publicKey: requestOptions } }),
      ),
      http.post("*/api/admin/v1/auth/login/finish", () =>
        HttpResponse.json(
          { error: { code: "webauthn_verification_failed", message: "x", request_id: "01J" } },
          { status: 422 },
        ),
      ),
    );

    await expect(loginWithPasskey({ autofill: false })).rejects.toMatchObject({
      status: 422,
      code: "webauthn_verification_failed",
    });
  });
});

describe("registerPasskeyBySetupLink", () => {
  it("begins setup by token, registers and finishes with the name", async () => {
    let finishBody: unknown;
    server.use(
      http.post("*/api/admin/v1/auth/setup/tok_en/begin", () =>
        HttpResponse.json({ ceremony_id: CEREMONY, options: { publicKey: creationOptions } }),
      ),
      http.post("*/api/admin/v1/auth/setup/tok_en/finish", async ({ request }) => {
        finishBody = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );

    await registerPasskeyBySetupLink("tok_en", "MacBook");

    expect(startRegistration).toHaveBeenCalledExactlyOnceWith({ optionsJSON: creationOptions });
    expect(finishBody).toEqual({ ceremony_id: CEREMONY, credential: attestation, name: "MacBook" });
  });

  it("propagates 410 of beginSetup without calling the authenticator", async () => {
    server.use(
      http.post("*/api/admin/v1/auth/setup/old/begin", () =>
        HttpResponse.json(
          { error: { code: "setup_token_expired", message: "x", request_id: "01J" } },
          { status: 410 },
        ),
      ),
    );

    const promise = registerPasskeyBySetupLink("old", "MacBook");
    await expect(promise).rejects.toBeInstanceOf(ApiError);
    await expect(promise).rejects.toMatchObject({ status: 410 });
    expect(startRegistration).not.toHaveBeenCalled();
  });
});

describe("error helpers", () => {
  const aborted = new WebAuthnError({
    message: "aborted",
    code: "ERROR_CEREMONY_ABORTED",
    cause: new DOMException("aborted", "AbortError"),
  });
  const notAllowed = new DOMException(
    "The operation either timed out or was not allowed.",
    "NotAllowedError",
  );

  it("detects an aborted ceremony", () => {
    expect(isCeremonyAborted(aborted)).toBe(true);
    expect(isCeremonyAborted(new DOMException("aborted", "AbortError"))).toBe(true);
    expect(isCeremonyAborted(notAllowed)).toBe(false);
    expect(isCeremonyAborted(new Error("x"))).toBe(false);
  });

  it("treats 410 and 404 of the setup link as expired", () => {
    expect(isSetupLinkExpired(new ApiError(410, "setup_token_expired", "x", "r"))).toBe(true);
    expect(isSetupLinkExpired(new ApiError(404, "not_found", "x", "r"))).toBe(true);
    expect(isSetupLinkExpired(new ApiError(422, "webauthn_verification_failed", "x", "r"))).toBe(
      false,
    );
    expect(isSetupLinkExpired(new Error("x"))).toBe(false);
  });

  it.each([
    [
      "login",
      notAllowed,
      "Не удалось войти. Подтверждение отменено или ключ не подошёл. Попробуйте ещё раз.",
    ],
    [
      "login",
      new ApiError(422, "webauthn_verification_failed", "x", "r"),
      "Не удалось войти. Подтверждение отменено или ключ не подошёл. Попробуйте ещё раз.",
    ],
    ["setup", notAllowed, "Не удалось создать passkey. Попробуйте ещё раз."],
    [
      "setup",
      new ApiError(422, "webauthn_verification_failed", "x", "r"),
      "Не удалось создать passkey. Попробуйте ещё раз.",
    ],
    [
      "login",
      new ApiError(0, "network_error", "x", ""),
      "Нет соединения с сервером. Проверьте подключение.",
    ],
    [
      "setup",
      new ApiError(0, "network_error", "x", ""),
      "Нет соединения с сервером. Проверьте подключение.",
    ],
    [
      "login",
      new ApiError(429, "rate_limited", "x", "r"),
      "Слишком много попыток. Повторите через минуту.",
    ],
    [
      "login",
      new ApiError(403, "csrf_check_failed", "x", "r"),
      "Сессия устарела. Обновите страницу.",
    ],
  ] as const)("%s: %s → %s", (flow, err, message) => {
    expect(passkeyErrorMessage(err, flow)).toBe(message);
  });
});
