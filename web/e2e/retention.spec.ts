import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// S-06 retention (T-039). Данные — `make seed` в базу e2e: приложение «Seed iOS» с 35 днями событий
// по вчерашний день. Владелец уже вошёл: storageState из global-setup.

type AppList = { items: { id: string; name: string }[] };

async function seedAppId(page: Page): Promise<string> {
  const res = await page.request.get("/api/v1/apps");
  expect(res.ok()).toBe(true);
  const app = ((await res.json()) as AppList).items.find((a) => a.name === "Seed iOS");
  if (!app) throw new Error("Нет приложения «Seed iOS»: выполните `make seed` для базы e2e");
  return app.id;
}

test("switches cohort granularity", async ({ page }) => {
  const appId = await seedAppId(page);
  await page.goto(`/apps/${appId}/retention`);
  await expect(page.getByRole("heading", { level: 1, name: "Seed iOS" })).toBeVisible();

  const days = page.getByRole("table", {
    name: "Retention по дням: доля когорты, вернувшейся в день N",
  });
  await expect(days.getByRole("columnheader", { name: "D30", exact: true })).toBeVisible();
  // Seed пишет события каждый день: у старых когорт есть ненулевое удержание.
  await expect(days.locator("td[data-step]").first()).toBeVisible();
  await expect(page.getByRole("img", { name: /^Средняя кривая удержания по дням/ })).toBeVisible();

  await expectNoA11yViolations(page);

  await page
    .getByRole("radiogroup", { name: "Когорты" })
    .getByRole("radio", { name: "по неделям" })
    .click();
  await expect(page).toHaveURL(/[?&]cohort=week/);

  const weeks = page.getByRole("table", {
    name: "Retention по неделям: доля когорты, вернувшейся в неделю N",
  });
  await expect(weeks.getByRole("columnheader", { name: "W0", exact: true })).toBeVisible();
  await expect(weeks.getByRole("columnheader", { name: "W12", exact: true })).toBeAttached();
  await expect(weeks.getByRole("columnheader", { name: "W13", exact: true })).toHaveCount(0);
  await expect(weeks.getByRole("columnheader", { name: "D1", exact: true })).toHaveCount(0);

  await page.reload();
  await expect(weeks.getByRole("columnheader", { name: "W12", exact: true })).toBeAttached();

  await page.goBack();
  await expect(page).not.toHaveURL(/cohort=/);
  await expect(days.getByRole("columnheader", { name: "D30", exact: true })).toBeVisible();
});
