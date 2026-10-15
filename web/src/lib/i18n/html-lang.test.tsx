import { render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { HtmlLang } from "./html-lang";
import { langInitScript } from "./lang-script";

function runAt(path: string, cookie = "") {
  window.history.pushState({}, "", path);
  // biome-ignore lint/suspicious/noDocumentCookie: тест выставляет cookie, которую читает inline-скрипт
  document.cookie = "am_lang=; max-age=0; path=/";
  // biome-ignore lint/suspicious/noDocumentCookie: то же — сохранённый выбор языка
  if (cookie) document.cookie = `${cookie}; path=/`;
  document.documentElement.lang = "";
  new Function(langInitScript)();
  return document.documentElement.lang;
}

describe("langInitScript", () => {
  afterEach(() => window.history.pushState({}, "", "/"));

  it("takes the language of public pages from the address", () => {
    expect(runAt("/ru")).toBe("ru");
    expect(runAt("/ru/docs/recipes/curl", "am_lang=en")).toBe("ru");
    expect(runAt("/", "am_lang=ru")).toBe("en");
    expect(runAt("/docs/recipes/curl", "am_lang=ru")).toBe("en");
  });

  it("takes the language of the panel from the saved choice", () => {
    expect(runAt("/portfolio", "am_lang=ru")).toBe("ru");
    expect(runAt("/apps/x/retention")).toBe("en");
  });
});

describe("HtmlLang", () => {
  it("sets the lang attribute after hydration", () => {
    document.documentElement.lang = "en";
    render(<HtmlLang locale="ru" />);
    expect(document.documentElement.lang).toBe("ru");
  });
});
