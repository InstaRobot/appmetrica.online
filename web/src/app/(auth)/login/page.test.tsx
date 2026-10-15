import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Locale } from "@/lib/i18n/locale";
import { generateMetadata } from "./page";

const request = vi.hoisted(() => ({ locale: "en" as Locale }));
vi.mock("@/lib/i18n/server", () => ({ getRequestLocale: async () => request.locale }));

beforeEach(() => {
  request.locale = "en";
});

// Язык входа — из cookie и Accept-Language запроса (getRequestLocale).
describe("login metadata", () => {
  it("names the page in the language of the request", async () => {
    expect(await generateMetadata()).toEqual({ title: "Sign in" });
    request.locale = "ru";
    expect(await generateMetadata()).toEqual({ title: "Вход" });
  });
});
