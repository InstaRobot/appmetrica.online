import { beforeEach, describe, expect, it, vi } from "vitest";

const request = vi.hoisted(() => ({ cookie: undefined as string | undefined, acceptLanguage: "" }));

vi.mock("next/headers", () => ({
  cookies: async () => ({
    get: (name: string) =>
      name === "am_lang" && request.cookie !== undefined ? { value: request.cookie } : undefined,
  }),
  headers: async () => new Headers({ "accept-language": request.acceptLanguage }),
}));

import { getRequestLocale } from "./server";

describe("getRequestLocale", () => {
  beforeEach(() => {
    request.cookie = undefined;
    request.acceptLanguage = "";
  });

  it("reads the saved choice first", async () => {
    request.cookie = "ru";
    request.acceptLanguage = "en-US";
    expect(await getRequestLocale()).toBe("ru");
  });

  it("falls back to the browser language and then to English", async () => {
    request.acceptLanguage = "ru-RU,ru;q=0.9";
    expect(await getRequestLocale()).toBe("ru");
    request.acceptLanguage = "de-DE";
    expect(await getRequestLocale()).toBe("en");
  });
});
