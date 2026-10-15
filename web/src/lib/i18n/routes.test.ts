import { describe, expect, it } from "vitest";
import { isPublicPath, publicHref, switchLocaleHref } from "./routes";

describe("publicHref", () => {
  it("keeps English without a prefix and puts Russian under /ru", () => {
    expect(publicHref("en", "/")).toBe("/");
    expect(publicHref("en", "/docs/recipes/curl")).toBe("/docs/recipes/curl");
    expect(publicHref("ru", "/")).toBe("/ru");
    expect(publicHref("ru", "/docs")).toBe("/ru/docs");
  });
});

describe("isPublicPath", () => {
  it.each([
    ["/", true],
    ["/ru", true],
    ["/docs/recipes/curl", true],
    ["/ru/docs", true],
    ["/portfolio", false],
    ["/russia", false],
  ])("%s → %s", (path, want) => {
    expect(isPublicPath(path)).toBe(want);
  });
});

describe("switchLocaleHref", () => {
  it("returns the paired address of a public page", () => {
    expect(switchLocaleHref("/", "ru")).toBe("/ru");
    expect(switchLocaleHref("/ru", "en")).toBe("/");
    expect(switchLocaleHref("/docs/recipes/curl", "ru")).toBe("/ru/docs/recipes/curl");
    expect(switchLocaleHref("/ru/docs/recipes/curl", "en")).toBe("/docs/recipes/curl");
  });

  it("keeps panel addresses: the language of the panel lives in the cookie", () => {
    expect(switchLocaleHref("/portfolio", "ru")).toBeNull();
  });
});
