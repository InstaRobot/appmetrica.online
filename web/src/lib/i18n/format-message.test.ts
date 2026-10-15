import { describe, expect, it } from "vitest";
import { fmt, plural } from "./format-message";

describe("fmt", () => {
  it("substitutes named placeholders and keeps unknown ones", () => {
    expect(fmt("Удалить {name}?", { name: "Notes" })).toBe("Удалить Notes?");
    expect(fmt("{a} и {a}", { a: 1 })).toBe("1 и 1");
    expect(fmt("Нет {missing}", {})).toBe("Нет {missing}");
    expect(fmt("Без подстановок")).toBe("Без подстановок");
  });
});

describe("plural", () => {
  const ru = {
    one: "{count} событие",
    few: "{count} события",
    many: "{count} событий",
    other: "{count} события",
  };
  const en = { one: "{count} event", other: "{count} events" };

  it("picks the Russian form by CLDR rules and formats the count", () => {
    expect(plural("ru", ru, 1)).toBe("1 событие");
    expect(plural("ru", ru, 3)).toBe("3 события");
    expect(plural("ru", ru, 11)).toBe("11 событий");
    expect(plural("ru", ru, 21)).toBe("21 событие");
    expect(plural("ru", ru, 1.5)).toBe("1,5 события");
    expect(plural("ru", ru, 12345)).toBe("12 345 событий");
  });

  it("picks the English form and formats the count with en-US grouping", () => {
    expect(plural("en", en, 1)).toBe("1 event");
    expect(plural("en", en, 0)).toBe("0 events");
    expect(plural("en", en, 12345)).toBe("12,345 events");
  });

  it("falls back to other when a form is missing and passes extra placeholders", () => {
    expect(
      plural("ru", { one: "{count} из {total}", other: "{count} из {total}" }, 5, { total: 9 }),
    ).toBe("5 из 9");
  });
});
