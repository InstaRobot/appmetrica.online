import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Locale } from "@/lib/i18n/locale";
import { generateMetadata } from "./page";

const request = vi.hoisted(() => ({ locale: "en" as Locale }));
vi.mock("@/lib/i18n/server", () => ({ getRequestLocale: async () => request.locale }));

beforeEach(() => {
  request.locale = "en";
});

// Язык регистрации passkey — из cookie и Accept-Language запроса (getRequestLocale).
describe("passkey setup metadata", () => {
  it("names the page in the language of the request", async () => {
    expect(await generateMetadata()).toEqual({ title: "Passkey setup" });
    request.locale = "ru";
    expect(await generateMetadata()).toEqual({ title: "Регистрация passkey" });
  });
});
