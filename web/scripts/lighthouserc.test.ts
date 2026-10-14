import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

// AC-044.3: бюджеты NFR-006 для портфеля в web/lighthouserc.json. INP в навигационном прогоне
// Lighthouse не измеряется — его лабораторный заменитель Total Blocking Time ≤ 200 мс.
type Assertion = [string, { maxNumericValue?: number }];
type LhciConfig = {
  ci: {
    collect: { url: string[]; numberOfRuns?: number; settings?: { preset?: string } };
    assert: { assertions: Record<string, Assertion> };
    upload: { target: string };
  };
};

const config = JSON.parse(
  readFileSync(path.resolve(import.meta.dirname, "../lighthouserc.json"), "utf8"),
) as LhciConfig;
const { collect, assert, upload } = config.ci;

describe("lighthouserc.json", () => {
  it("audits the portfolio page on desktop", () => {
    expect(collect.url.map((url) => new URL(url).pathname)).toEqual(["/"]);
    expect(collect.settings?.preset).toBe("desktop");
    expect(collect.numberOfRuns).toBeGreaterThanOrEqual(3);
  });

  it.each([
    ["largest-contentful-paint", 2500],
    ["total-blocking-time", 200],
  ])("fails the build when %s exceeds %i ms", (audit, max) => {
    expect(assert.assertions[audit]).toEqual(["error", { maxNumericValue: max }]);
  });

  it("requires CLS below 0.1", () => {
    const [level, options] = assert.assertions["cumulative-layout-shift"] ?? [];
    expect(level).toBe("error");
    expect(options?.maxNumericValue).toBeLessThan(0.1);
  });

  it("keeps reports of the owner's dashboard off public storage", () => {
    expect(upload.target).toBe("filesystem");
  });
});
