import AxeBuilder from "@axe-core/playwright";
import { expect, type Page } from "@playwright/test";

// Правила axe-core для WCAG 2.2 AA (NFR-008, 22-test-strategy.md §12): критерии A и AA версий 2.0–2.2.
export const WCAG_22_AA_TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

// expectNoA11yViolations проверяет текущее состояние страницы axe-core и падает со списком нарушений:
// id правила, описание и CSS-селекторы элементов. Вызывается в каждой E2E-спеке после загрузки
// экрана (T-044, экраны S-01…S-14).
export async function expectNoA11yViolations(page: Page): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG_22_AA_TAGS).analyze();
  const found = violations.map(
    (v) =>
      `${v.id} (${v.impact ?? "n/a"}): ${v.help} — ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`,
  );
  expect(found, `нарушения WCAG 2.2 AA на ${page.url()}`).toEqual([]);
}
