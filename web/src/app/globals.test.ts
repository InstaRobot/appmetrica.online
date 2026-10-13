// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(join(import.meta.dirname, "globals.css"), "utf8");

const declared = (name: string) => new RegExp(`${name.replace(/[-]/g, "\\-")}\\s*:`).test(css);

describe("globals.css design tokens", () => {
  it.each([
    "--bg-page",
    "--bg-base",
    "--bg-subtle",
    "--bg-muted",
    "--bg-selected",
    "--bg-inverse",
    "--fg-base",
    "--fg-muted",
    "--fg-subtle",
    "--fg-inverse",
    "--fg-link",
    "--fg-positive",
    "--fg-negative",
    "--fg-warning",
    "--border-base",
    "--border-strong",
    "--ring-focus",
  ])("declares semantic alias %s", (name) => {
    expect(declared(name)).toBe(true);
  });

  it.each([1, 2, 3, 4, 5, 6, 7, 8])("maps --color-chart-%d in @theme", (n) => {
    expect(declared(`--chart-${n}`)).toBe(true);
    expect(css).toContain(`--color-chart-${n}: var(--chart-${n});`);
  });

  it.each([0, 1, 2, 3, 4, 5])("declares retention step --ret-%d", (n) => {
    expect(css).toContain(`--color-ret-${n}: var(--ret-${n});`);
  });

  it("keeps the light-only theme and 4px spacing", () => {
    expect(css).toContain("color-scheme: light");
    expect(css).toContain("--spacing: 4px;");
    expect(css).not.toContain("prefers-color-scheme");
  });

  it.each([
    "text-kpi-xl",
    "text-kpi-lg",
    "text-heading-1",
    "text-heading-2",
    "text-heading-3",
    "text-body-md",
    "text-body-sm",
    "text-label-md",
    "text-label-sm",
    "text-caption",
    "text-code",
    "z-sticky",
    "z-dropdown",
    "z-overlay",
    "z-modal",
    "z-toast",
    "z-tooltip",
    "z-skiplink",
  ])("declares @utility %s", (name) => {
    expect(css).toContain(`@utility ${name} {`);
  });
});
