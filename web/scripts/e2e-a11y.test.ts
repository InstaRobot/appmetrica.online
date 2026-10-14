import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

// AC-044.1: каждая E2E-спека проверяет свои экраны общим хелпером e2e/helpers/a11y.ts,
// чтобы набор правил axe (WCAG 2.2 AA) был один на все экраны S-01…S-14.
const E2E_DIR = path.resolve(import.meta.dirname, "../e2e");
const specs = readdirSync(E2E_DIR)
  .filter((name) => name.endsWith(".spec.ts"))
  .map((name) => ({ name, source: readFileSync(path.join(E2E_DIR, name), "utf8") }));

describe("e2e accessibility checks", () => {
  it("finds the e2e specs", () => {
    expect(specs.length).toBeGreaterThan(0);
  });

  it.each(specs)("$name calls expectNoA11yViolations from helpers/a11y", ({ source }) => {
    expect(source).toMatch(
      /import \{[^}]*\bexpectNoA11yViolations\b[^}]*\} from "\.\/helpers\/a11y";/,
    );
    expect(source).toMatch(/await expectNoA11yViolations\(page\b/);
  });

  it.each(specs)("$name does not run axe on its own", ({ source }) => {
    expect(source).not.toContain("@axe-core/playwright");
    expect(source).not.toMatch(/function expectNoA11yViolations/);
  });

  it("the helper checks WCAG 2.2 AA rules", () => {
    const helper = readFileSync(path.join(E2E_DIR, "helpers/a11y.ts"), "utf8");
    for (const tag of ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"]) {
      expect(helper).toContain(`"${tag}"`);
    }
  });
});
