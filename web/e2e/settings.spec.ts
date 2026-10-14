import { randomUUID } from "node:crypto";
import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";

// S-11 настройки приложения и S-12 глобальные настройки (T-043) против локального api с APP_ENV=e2e.
// Ответ Telegram в тесте отправки подменяется через page.route: у e2e-стенда нет бота.

type AppList = { items: { id: string; name: string }[] };

async function seedAppId(page: Page): Promise<string> {
  const res = await page.request.get("/api/v1/apps");
  expect(res.ok()).toBe(true);
  const app = ((await res.json()) as AppList).items.find((a) => a.name === "Seed iOS");
  if (!app) throw new Error("Нет приложения «Seed iOS»: выполните `make seed` для базы e2e");
  return app.id;
}

test("api key secret is shown once", async ({ page }) => {
  await page.goto("/settings?tab=api-keys");
  await expect(page.getByRole("tab", { name: "API-ключи" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  const label = `e2e-${randomUUID().slice(0, 8)}`;

  await page.getByRole("button", { name: "Создать ключ" }).first().click();
  let dialog = page.getByRole("dialog", { name: "Новый API-ключ" });
  await dialog.getByRole("textbox", { name: /Метка/ }).fill(label);
  await dialog.getByRole("button", { name: "Создать" }).click();

  const secretField = dialog.getByRole("textbox", { name: "секрет" });
  await expect(secretField).toHaveValue(/^sk_[A-Za-z0-9]{40}$/);
  const secret = await secretField.inputValue();
  await expect(dialog).toContainText("Секрет показывается один раз.");
  await expectNoA11yViolations(page);

  await dialog.getByRole("button", { name: "Я сохранил ключ" }).click();
  await expect(dialog).toBeHidden();

  const row = page.getByRole("table", { name: "API-ключи" }).getByRole("row", { name: label });
  await expect(row).toContainText(`${secret.slice(0, 11)}…`);
  expect(await page.content()).not.toContain(secret);

  await page.getByRole("button", { name: "Создать ключ" }).first().click();
  dialog = page.getByRole("dialog", { name: "Новый API-ключ" });
  await expect(dialog.getByRole("textbox", { name: /Метка/ })).toHaveValue("");
  await expect(dialog.getByRole("textbox", { name: "секрет" })).toHaveCount(0);
  await dialog.getByRole("button", { name: "Отмена" }).click();

  await row.getByRole("button", { name: /Отозвать ключ/ }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Отозвать" }).click();
  await expect(row).toContainText("Отозван");
});

test("telegram test shows error from telegram", async ({ page }) => {
  await page.route("**/admin/v1/settings/telegram/test", (route) =>
    route.fulfill({ json: { sent: false, error: "chat not found" } }),
  );
  await page.goto("/settings?tab=telegram");

  await expect(page.getByRole("textbox", { name: "Chat ID" })).toBeVisible();
  await expectNoA11yViolations(page);

  // Новый chat_id при каждом прогоне: иначе форма без изменений и «Сохранить» недоступна.
  const chat = page.getByRole("textbox", { name: "Chat ID" });
  await chat.fill(String(Date.now()));
  await page.getByRole("button", { name: "Сохранить" }).click();
  await expect(page.getByText("Настройки сохранены")).toBeVisible();

  await page.getByRole("button", { name: "Отправить тестовое сообщение" }).click();
  const alert = page.getByRole("alert").filter({ hasText: "Telegram не принял сообщение" });
  await expect(alert).toContainText("chat not found");
  await expect(page.getByText("Сообщение отправлено")).toHaveCount(0);
});

test("settings tabs switch with the keyboard and keep ?tab=", async ({ page }) => {
  await page.goto("/settings");
  const passkeys = page.getByRole("tab", { name: "Passkeys" });
  await expect(passkeys).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("table", { name: "Passkeys" })).toContainText("E2E");

  // Radix переводит фокус на соседнюю вкладку в setTimeout: следующая стрелка — после смены вкладки.
  await passkeys.focus();
  for (const name of ["API-ключи", "Telegram", "Система"]) {
    await page.keyboard.press("ArrowRight");
    const tab = page.getByRole("tab", { name });
    await expect(tab).toBeFocused();
    await expect(tab).toHaveAttribute("aria-selected", "true");
  }
  await expect(page).toHaveURL(/\?tab=system$/);
  await expect(page.getByRole("group", { name: "Postgres" })).toContainText("OK");
  await expectNoA11yViolations(page);

  await page.keyboard.press("ArrowRight");
  await expect(page).toHaveURL(/\?tab=deletions$/);
  await expect(page.getByRole("tabpanel", { name: "Удаления" })).toBeVisible();
});

test("app settings create a write key for a platform and revoke it", async ({ page }) => {
  const appId = await seedAppId(page);
  await page.goto(`/apps/${appId}/settings`);
  await expect(page.getByRole("textbox", { name: /Название/ })).toHaveValue("Seed iOS");
  const basics = page.getByRole("form", { name: "Основное" });
  await expect(basics.getByRole("button", { name: "Сохранить" })).toBeDisabled();
  const platforms = page.getByRole("region", { name: "Платформы" });
  await expect(platforms.getByRole("button", { name: "iOS" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(platforms.getByRole("button", { name: "Сохранить" })).toBeDisabled();
  await expectNoA11yViolations(page);

  const label = `e2e-${randomUUID().slice(0, 8)}`;
  await page.getByRole("button", { name: "Создать ключ" }).click();
  const dialog = page.getByRole("dialog", { name: "Новый write-ключ" });
  await dialog.getByRole("textbox", { name: "Метка" }).fill(label);
  const platform = dialog.getByRole("combobox", { name: "Платформа" });
  await expect(platform).toHaveText("Любая платформа");
  await expectNoA11yViolations(page);
  await platform.click();
  await page.getByRole("option", { name: "Android" }).click();
  await dialog.getByRole("button", { name: "Создать" }).click();
  await expect(dialog.getByRole("textbox", { name: "write-ключ" })).toHaveValue(
    /^pk_[A-Za-z0-9]{32}$/,
  );
  await dialog.getByRole("button", { name: "Готово" }).click();

  const row = page.getByRole("table", { name: "Write-ключи" }).getByRole("row", { name: label });
  await expect(row).toContainText("Android");
  await row.getByRole("button", { name: /Отозвать ключ/ }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Отозвать" }).click();
  await expect(row).toContainText("Отозван");
});
