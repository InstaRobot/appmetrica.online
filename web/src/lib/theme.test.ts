import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyTheme, readTheme, THEME_STORAGE_KEY, themeInitScript } from "./theme";

const root = () => document.documentElement;

function mockSystemTheme(dark: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: dark && query === "(prefers-color-scheme: dark)",
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
}

// Скрипт вставляется в <head> строкой и выполняется до первой отрисовки (без мигания темы).
const runInitScript = () => new Function(themeInitScript)();

beforeEach(() => {
  localStorage.clear();
  delete root().dataset.theme;
  root().classList.remove("dark");
  root().style.colorScheme = "";
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("themeInitScript", () => {
  it("applies the theme saved in localStorage", () => {
    mockSystemTheme(false);
    localStorage.setItem(THEME_STORAGE_KEY, "dark");

    runInitScript();

    expect(root().dataset.theme).toBe("dark");
    expect(root().classList.contains("dark")).toBe(true);
    expect(root().style.colorScheme).toBe("dark");
  });

  it("follows the system preference when nothing is saved", () => {
    mockSystemTheme(true);

    runInitScript();

    expect(root().dataset.theme).toBe("dark");
  });

  it("falls back to light for an unknown saved value and a light system", () => {
    mockSystemTheme(false);
    localStorage.setItem(THEME_STORAGE_KEY, "sepia");

    runInitScript();

    expect(root().dataset.theme).toBe("light");
    expect(root().classList.contains("dark")).toBe(false);
    expect(root().style.colorScheme).toBe("light");
  });

  it("does not throw when storage is unavailable", () => {
    mockSystemTheme(false);
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("SecurityError");
    });

    expect(runInitScript).not.toThrow();
    expect(root().dataset.theme).toBe("light");
  });
});

describe("applyTheme / readTheme", () => {
  it("switches the document theme and remembers the choice", () => {
    applyTheme("dark");

    expect(readTheme()).toBe("dark");
    expect(root().classList.contains("dark")).toBe(true);
    expect(root().style.colorScheme).toBe("dark");
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");

    applyTheme("light");

    expect(readTheme()).toBe("light");
    expect(root().classList.contains("dark")).toBe(false);
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("light");
  });

  it("reads light when the attribute is missing", () => {
    expect(readTheme()).toBe("light");
  });
});
