import { describe, expect, it } from "vitest";
import { plural } from "./plural";

const forms = ["событие", "события", "событий"] as const;

describe("plural", () => {
  it.each([
    [0, "событий"],
    [1, "событие"],
    [2, "события"],
    [4, "события"],
    [5, "событий"],
    [11, "событий"],
    [12, "событий"],
    [14, "событий"],
    [21, "событие"],
    [22, "события"],
    [101, "событие"],
    [111, "событий"],
    [1_000_000, "событий"],
  ])("%d → %s", (n, want) => {
    expect(plural(n, forms)).toBe(want);
  });

  it("uses the absolute value and the many form for fractions", () => {
    expect(plural(-1, forms)).toBe("событие");
    expect(plural(1.5, forms)).toBe("события");
  });
});
