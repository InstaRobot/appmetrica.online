import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { ApiError } from "./api/fetcher";
import { API_ERRORS, GENERIC_ERROR, messageFor, toUiError } from "./errors";

// Коды из components.responses контракта: description вида «app_not_found | profile_not_found | not_found».
function contractErrorCodes(): string[] {
  const yaml = readFileSync(
    join(import.meta.dirname, "../../../contracts/openapi/api.yaml"),
    "utf8",
  );
  const responses = yaml.slice(yaml.indexOf("\n  responses:\n"), yaml.indexOf("\n  schemas:\n"));
  return [...responses.matchAll(/^ {6}description: (.+)$/gm)]
    .flatMap((m) => (m[1] ?? "").split("|").map((s) => s.trim()))
    .filter((code) => /^[a-z_]+$/.test(code));
}

describe("errors", () => {
  it("maps API error codes to russian messages", () => {
    expect(messageFor("app_not_found")).toBe("Приложение не найдено или удалено");
    expect(messageFor("range_too_large")).toBe(
      "Слишком большой период: максимум 366 дней, для детализации по часам — 7 дней.",
    );
    expect(messageFor("storage_unavailable")).toBe(
      "Хранилище данных временно недоступно. Повторим автоматически.",
    );
    expect(messageFor("something_new")).toBe(GENERIC_ERROR);
    expect(GENERIC_ERROR).toMatch(/^Что-то пошло не так/);

    const unknown = toUiError(new ApiError(418, "something_new", "teapot", "01JREQ"));
    expect(unknown).toEqual({
      code: "something_new",
      message: GENERIC_ERROR,
      requestId: "01JREQ",
      status: 418,
    });
  });

  it("has a message for every error code of the contract", () => {
    const codes = contractErrorCodes();
    expect(codes).toEqual(expect.arrayContaining(["app_not_found", "storage_unavailable"]));
    for (const code of codes) {
      expect(API_ERRORS, code).toHaveProperty(code);
      expect(messageFor(code), code).not.toBe(GENERIC_ERROR);
    }
  });

  it("uses server message for invalid_request and Retry-After for rate_limited", () => {
    expect(messageFor("invalid_request", { message: "from: invalid date" })).toBe(
      "Некорректный запрос: from: invalid date",
    );
    expect(messageFor("invalid_request")).toBe("Некорректный запрос.");
    expect(messageFor("rate_limited", { retryAfter: 12 })).toBe(
      "Слишком много запросов. Повторим через 12 с.",
    );
    expect(messageFor("rate_limited")).toBe("Слишком много запросов. Повторим автоматически.");
  });

  it("falls back to server error for unknown 5xx and to network text for fetch failures", () => {
    expect(toUiError(new ApiError(502, "http_502", "Bad Gateway", "")).message).toBe(
      "Ошибка сервера. Повторите через минуту.",
    );
    expect(toUiError(new ApiError(0, "network_error", "Нет соединения", ""))).toEqual({
      code: "network_error",
      message: "Нет соединения с сервером. Проверьте подключение.",
      status: 0,
    });
    expect(toUiError(new TypeError("Failed to fetch")).message).toBe(
      "Нет соединения с сервером. Проверьте подключение.",
    );
  });

  it("accepts error envelopes and arbitrary values", () => {
    expect(
      toUiError({ code: "not_found", message: "no such key", request_id: "01JX", status: 404 }),
    ).toEqual({
      code: "not_found",
      message: "Объект не найден или был удалён.",
      requestId: "01JX",
      status: 404,
    });
    expect(toUiError(new Error("boom"))).toEqual({ code: "unknown", message: GENERIC_ERROR });
    expect(toUiError("boom")).toEqual({ code: "unknown", message: GENERIC_ERROR });
  });
});
