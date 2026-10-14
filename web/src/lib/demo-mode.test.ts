import { afterEach, describe, expect, it } from "vitest";
import { DEMO_COOKIE, hasDemoCookie, isDemoInBrowser } from "./demo-mode";

describe("demo mode cookie", () => {
  afterEach(() => {
    // biome-ignore lint/suspicious/noDocumentCookie: тест эмулирует cookie демо-режима
    document.cookie = `${DEMO_COOKIE}=; max-age=0; path=/`;
  });

  it("is a readable am_demo=1 cookie", () => {
    expect(DEMO_COOKIE).toBe("am_demo");
    expect(hasDemoCookie("am_session=x; am_demo=1")).toBe(true);
    expect(hasDemoCookie("am_demo=1")).toBe(true);
  });

  it.each([undefined, "", "am_demo=0", "am_demo=", "xam_demo=1", "am_demo=10"])(
    "does not treat %j as demo",
    (header) => {
      expect(hasDemoCookie(header)).toBe(false);
    },
  );

  it("reads the cookie in the browser", () => {
    expect(isDemoInBrowser()).toBe(false);

    // biome-ignore lint/suspicious/noDocumentCookie: тест эмулирует cookie демо-режима
    document.cookie = `${DEMO_COOKIE}=1; path=/`;

    expect(isDemoInBrowser()).toBe(true);
  });
});
