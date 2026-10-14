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

  it("keeps 4px spacing and switches themes only through data-theme", () => {
    expect(css).toContain("--spacing: 4px;");
    expect(css).toContain("color-scheme: light");
    expect(css).toContain("color-scheme: dark");
    // Тема выбирается скриптом lib/theme.ts (сохранённый выбор или системная), а не медиа-запросом в CSS.
    expect(css).not.toContain("prefers-color-scheme");
    expect(css).toContain(
      '@custom-variant dark (&:where([data-theme="dark"], [data-theme="dark"] *));',
    );
  });

  it.each([
    "--bg-page",
    "--bg-base",
    "--bg-subtle",
    "--bg-muted",
    "--bg-selected",
    "--bg-inverse",
    "--bg-header",
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
    "--primary",
    "--primary-top",
    "--primary-hover",
    "--chart-1",
    "--chart-compare",
    "--code-bg",
    "--code-fg",
    "--shadow-card",
    "--shadow-primary",
    "--glow",
  ])("dark theme redefines %s", (name) => {
    expect(tokens.dark).toHaveProperty(name);
  });
});

// ---- контраст WCAG 2.2 AA (NFR-008) для обеих тем -------------------------------------------

function block(selector: string): string {
  const start = css.indexOf(`${selector} {`);
  if (start < 0) throw new Error(`block ${selector} not found`);
  return css.slice(start, css.indexOf("\n}", start));
}

function declarations(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const m of text.matchAll(/(--[\w-]+):\s*([^;]+);/g)) {
    const [, name, value] = m;
    if (name && value) out[name] = value.trim();
  }
  return out;
}

const light = declarations(block(":root"));
const tokens = { light, dark: declarations(block(':root[data-theme="dark"]')) };

function resolve(theme: "light" | "dark", name: string, depth = 0): string {
  const value = (theme === "dark" ? tokens.dark[name] : undefined) ?? light[name];
  if (!value) throw new Error(`${theme}: token ${name} is not declared`);
  const ref = /^var\((--[\w-]+)\)$/.exec(value);
  if (ref?.[1] && depth < 10) return resolve(theme, ref[1], depth + 1);
  return value;
}

function luminance(hex: string): number {
  const m = /^#([0-9a-f]{6})$/i.exec(hex);
  if (!m?.[1]) throw new Error(`expected #rrggbb, got ${hex}`);
  const [r, g, b] = [0, 2, 4].map((i) => Number.parseInt(m[1]?.slice(i, i + 2) ?? "", 16) / 255);
  const lin = (c = 0) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

function contrast(theme: "light" | "dark", fg: string, bg: string): number {
  const [a, b] = [luminance(resolve(theme, fg)), luminance(resolve(theme, bg))];
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

const TEXT_PAIRS: [string, string][] = [
  ["--fg-base", "--bg-base"],
  ["--fg-base", "--bg-page"],
  ["--fg-base", "--bg-selected"],
  ["--fg-muted", "--bg-base"],
  ["--fg-muted", "--bg-page"],
  ["--fg-muted", "--bg-subtle"],
  ["--fg-subtle", "--bg-base"],
  ["--fg-subtle", "--bg-page"],
  ["--fg-subtle", "--bg-subtle"],
  ["--fg-link", "--bg-base"],
  ["--fg-link", "--bg-page"],
  ["--fg-link", "--bg-selected"],
  ["--fg-positive", "--bg-base"],
  ["--fg-positive", "--color-success-50"],
  ["--fg-negative", "--bg-base"],
  ["--fg-negative", "--color-danger-50"],
  ["--fg-warning", "--bg-base"],
  ["--fg-warning", "--color-warning-50"],
  ["--primary-foreground", "--primary"],
  ["--primary-foreground", "--primary-top"],
  ["--primary-foreground", "--primary-hover"],
  ["--accent-foreground", "--accent"],
  ["--code-fg", "--code-bg"],
  ["--ret-fg-0", "--ret-0"],
  ["--ret-fg-1", "--ret-1"],
  ["--ret-fg-2", "--ret-2"],
  ["--ret-fg-3", "--ret-3"],
  ["--ret-fg-4", "--ret-4"],
  ["--ret-fg-5", "--ret-5"],
];

// Нетекстовые элементы (WCAG 1.4.11): фокус-кольцо, основная серия графика, линия сравнения.
const UI_PAIRS: [string, string][] = [
  ["--ring-focus", "--bg-base"],
  ["--ring-focus", "--bg-page"],
  ["--chart-1", "--bg-base"],
  ["--chart-compare", "--bg-base"],
];

describe.each(["light", "dark"] as const)("%s theme contrast", (theme) => {
  it.each(TEXT_PAIRS)("text %s on %s ≥ 4.5:1", (fg, bg) => {
    expect(contrast(theme, fg, bg)).toBeGreaterThanOrEqual(4.5);
  });

  it.each(UI_PAIRS)("ui %s on %s ≥ 3:1", (fg, bg) => {
    expect(contrast(theme, fg, bg)).toBeGreaterThanOrEqual(3);
  });
});

describe("globals.css status palette", () => {
  // Сырая палитра объявлена в :root (разная для тем), поэтому Tailwind не видит её без @theme:
  // bg-success-500 и подобные классы молча не генерировались. reference — без повторного вывода
  // переменных (иначе в слое theme появилось бы циклическое --color-x: var(--color-x)).
  const reference = css.slice(css.indexOf("@theme reference {"));

  it.each(
    ["success", "warning", "danger"].flatMap((tone) =>
      [50, 500, 600, 700].map((step) => `--color-${tone}-${step}`),
    ),
  )("exposes %s to Tailwind utilities", (name) => {
    expect(css).toContain("@theme reference {");
    expect(reference).toContain(`${name}: var(${name});`);
  });
});

describe("globals.css utilities", () => {
  // Цвет --color-X порождает утилиты text-X/bg-X: если X совпадает с типографской @utility text-X,
  // классы сливаются (так text-code однажды получил цвет фона блока кода).
  it("keeps @theme color names apart from text-* utilities", () => {
    const utilities = [...css.matchAll(/@utility text-([\w-]+) \{/g)].map((m) => m[1]);
    for (const name of utilities) {
      expect(css, `--color-${name} collides with @utility text-${name}`).not.toMatch(
        new RegExp(`--color-${name}:`),
      );
    }
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
