import { describe, expect, it } from "vitest";
import { safeNext } from "./safe-next";

describe("safeNext", () => {
  it("sends the root and empty next to the portfolio instead of the public landing", () => {
    expect(safeNext("/")).toBe("/portfolio");
    expect(safeNext(undefined)).toBe("/portfolio");
    expect(safeNext("/?utm=1")).toBe("/portfolio");
  });

  it("rejects absolute and protocol-relative next", () => {
    expect(safeNext("https://evil.example")).toBe("/portfolio");
    expect(safeNext("//evil.example")).toBe("/portfolio");
    expect(safeNext("/apps/1")).toBe("/apps/1");
  });

  it("keeps query and hash of relative paths", () => {
    expect(safeNext("/apps/1/retention?from=2026-10-01#heatmap")).toBe(
      "/apps/1/retention?from=2026-10-01#heatmap",
    );
  });

  it.each([
    ["missing", undefined],
    ["null", null],
    ["empty", ""],
    ["backslash host", "/\\evil.example"],
    ["double backslash", "\\\\evil.example"],
    ["javascript scheme", "javascript:alert(1)"],
    ["data scheme", "data:text/html,hi"],
    ["relative without slash", "apps/1"],
    ["tab inside", "/\t/evil.example"],
    ["newline inside", "/\n/evil.example"],
    ["leading space", " /apps/1"],
    ["login loop", "/login?next=%2Fapps"],
    ["dot segments to protocol-relative", "/..//evil.example"],
    ["nested dot segments to protocol-relative", "/a/..//evil.example"],
    ["encoded dot segments to protocol-relative", "/%2e%2e//evil.example"],
    ["dot segments to backslash host", "/..//\\evil.example"],
  ])("falls back to /portfolio for %s", (_name, value) => {
    expect(safeNext(value)).toBe("/portfolio");
  });

  it("takes the first value of an array searchParam", () => {
    expect(safeNext(["/apps/2", "/apps/3"])).toBe("/apps/2");
    expect(safeNext([])).toBe("/portfolio");
  });
});
