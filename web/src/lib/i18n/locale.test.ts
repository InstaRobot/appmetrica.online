import { describe, expect, it } from "vitest";
import { isLocale, localeFromAcceptLanguage, resolveLocale } from "./locale";

describe("isLocale", () => {
  it("accepts only en and ru", () => {
    expect(isLocale("en")).toBe(true);
    expect(isLocale("ru")).toBe(true);
    expect(isLocale("de")).toBe(false);
    expect(isLocale(undefined)).toBe(false);
  });
});

describe("localeFromAcceptLanguage", () => {
  it.each([
    ["ru-RU,ru;q=0.9,en-US;q=0.8", "ru"],
    ["en-US,en;q=0.9,ru;q=0.8", "en"],
    ["de-DE,ru;q=0.7,en;q=0.5", "ru"],
    ["de-DE,en;q=0.7,ru;q=0.5", "en"],
    ["uk-UA,ru;q=0.9", "ru"],
    ["en;q=0.4,ru;q=0.6", "ru"],
    ["ru;q=0", null],
    ["fr-FR", null],
    ["", null],
  ])("%s → %s", (header, want) => {
    expect(localeFromAcceptLanguage(header)).toBe(want);
  });
});

describe("resolveLocale", () => {
  it("prefers the saved cookie, then the browser language, then English", () => {
    expect(resolveLocale({ cookie: "ru", acceptLanguage: "en-US" })).toBe("ru");
    expect(resolveLocale({ cookie: "en", acceptLanguage: "ru-RU" })).toBe("en");
    expect(resolveLocale({ cookie: "xx", acceptLanguage: "ru-RU" })).toBe("ru");
    expect(resolveLocale({ acceptLanguage: "fr-FR" })).toBe("en");
    expect(resolveLocale({})).toBe("en");
  });
});
