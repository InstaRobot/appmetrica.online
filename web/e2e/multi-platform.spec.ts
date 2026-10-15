import { expect, type Page, test } from "@playwright/test";
import { expectNoA11yViolations } from "./helpers/a11y";
import { appIdByName, createApp, sendSession } from "./helpers/seed";

// Мультиплатформенные приложения: одно приложение на iOS, Android и Web,
// переключатель платформ над отчётами, доли платформ в портфеле и слияние приложений-дублей.
// Нужны ingest (E2E_INGEST_URL) и worker с app_merge и MERGE_KEYS_WAIT=0 (APP_ENV=e2e): иначе шаг wait_keys
// ждёт 17 минут. «Seed Notes» — из `make seed`. Владелец уже вошёл: storageState из global-setup.

// Имена уникальны на прогон: база e2e переживает прогоны, а имена приложений уникальны без учёта регистра.
const run = Date.now().toString(36);

const PLATFORMS = ["iOS", "Android", "Веб"];

type Realtime = { recent_events: { install_id: string; platform: string }[] };

const activeValue = (page: Page) =>
  page
    .getByRole("region", { name: "Ключевые метрики" })
    .getByRole("button", { name: /^Активные/ })
    .locator("[data-kpi-value]");

test("switches reports between platforms", async ({ page }) => {
  // Новые ключи ingest видит после перезагрузки кэша (до 15 с).
  test.setTimeout(90_000);
  const name = `E2E Notes ${run}`;

  await test.step("create an app with three platforms", async () => {
    await page.goto("/apps/new");
    await page.getByRole("textbox", { name: "Название" }).fill(name);
    const group = page.getByRole("group", { name: "Платформы" });
    for (const platform of PLATFORMS) {
      const toggle = group.getByRole("button", { name: platform, exact: true });
      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-pressed", "true");
    }
    await expectNoA11yViolations(page);
    await page.getByRole("button", { name: "Создать" }).click();
    await expect(page).toHaveURL(/\/apps\/[0-9a-f-]{36}\/integration$/);
  });

  await test.step("integration has a section per platform", async () => {
    await expect(page.getByRole("heading", { level: 3 })).toHaveText(["Эндпоинт", ...PLATFORMS]);
    for (const platform of PLATFORMS) {
      await expect(
        page
          .getByRole("region", { name: platform })
          .getByRole("status", { name: `Статус ${platform}` }),
      ).toContainText(`Ждём первое событие с ${platform}`);
    }
    await expectNoA11yViolations(page);
  });

  await test.step("send one session per platform key", async () => {
    for (const platform of PLATFORMS) {
      const section = page.getByRole("region", { name: platform });
      const key = await section.getByRole("textbox", { name: "write-ключ" }).inputValue();
      // Без context.platform: платформа события берётся из ключа секции.
      await sendSession(page.request, key);
    }
    // Статус секции — по breakdown платформ за 48 ч, опрос раз в 3 с: события уже в ClickHouse.
    for (const platform of PLATFORMS) {
      await expect(
        page
          .getByRole("region", { name: platform })
          .getByRole("status", { name: `Статус ${platform}` }),
      ).toContainText("Получено первое событие", { timeout: 15_000 });
    }
  });

  await test.step("filter by platform and keep it across tabs", async () => {
    await page.getByRole("link", { name: "Аудитория" }).click();
    await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
    const switcher = page.getByRole("radiogroup", { name: "Платформа" });
    await expect(switcher.getByRole("radio")).toHaveText(["Все", ...PLATFORMS]);
    await expect(switcher.getByRole("radio", { name: "Все" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await expect(activeValue(page)).toHaveText("3");

    await switcher.getByRole("radio", { name: "Android" }).click();
    await expect(page).toHaveURL(/[?&]platform=android/);
    await expect(switcher.getByRole("radio", { name: "Android" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await expect(activeValue(page)).toHaveText("1");
    await expectNoA11yViolations(page);

    await page.getByRole("link", { name: "Retention" }).click();
    await expect(page).toHaveURL(/\/retention\?(.*&)?platform=android/);
    await expectNoA11yViolations(page);
  });

  await test.step("chart by platform", async () => {
    await page.getByRole("link", { name: "Аудитория" }).click();
    // Прежний маршрут остаётся в DOM скрытым: кликать по переключателю — после появления страницы аудитории.
    await expect(page.getByRole("region", { name: "Ключевые метрики" })).toBeVisible();
    await page
      .getByRole("radiogroup", { name: "Платформа" })
      .getByRole("radio", { name: "Все" })
      .click();
    await expect(page).not.toHaveURL(/platform=/);
    await page
      .getByRole("radiogroup", { name: "Режим графика" })
      .getByRole("radio", { name: "По платформам" })
      .click();
    await expect(page).toHaveURL(/[?&]split=platform/);
    await expect(
      page.getByRole("img", { name: /^График активных пользователей по дням по платформам/ }),
    ).toHaveAttribute("aria-label", /(?=.*iOS: )(?=.*Android: )(?=.*Веб: )/);
    await expectNoA11yViolations(page);
  });
});

test("shows platform shares in the portfolio", async ({ page }) => {
  await page.goto("/portfolio");
  const table = page.getByRole("table", { name: /Портфель приложений за/ });
  const row = table
    .getByRole("row")
    .filter({ has: page.getByRole("link", { name: "Seed Notes", exact: true }) });
  await expect(row, "нет «Seed Notes»: выполните `make seed` для базы e2e").toHaveCount(1);
  await expect(row.getByTitle(PLATFORMS.join(", "), { exact: true })).toBeVisible();

  // 5 % людей «Seed Notes» — на iOS и Web: сумма по платформам больше активных за день.
  const shares = row.getByRole("img", { name: /iOS \d+/ });
  await expect(shares).toHaveAttribute("aria-label", /Android \d+/);
  await expect(shares).toHaveAttribute("aria-label", /Веб \d+/);
  await shares.focus();
  await expect(page.getByRole("tooltip")).toContainText(
    "Человек на нескольких платформах учтён в каждой",
  );
  await expectNoA11yViolations(page);
});

test("merges two apps", async ({ page }) => {
  // Перезагрузка кэша ключей ingest (до 15 с) и запуск app_merge worker-а раз в минуту.
  test.setTimeout(240_000);
  const targetName = `E2E iOS ${run}`;
  const sourceName = `E2E Android ${run}`;
  const target = await createApp(page.request, targetName, ["ios"]);
  const source = await createApp(page.request, sourceName, ["android"]);
  await sendSession(page.request, target.key("ios"));
  await sendSession(page.request, source.key("android"));

  await test.step("merge from the target settings", async () => {
    await page.goto(`/apps/${target.id}/settings`);
    await page.getByRole("button", { name: "Объединить…" }).click();
    const dialog = page.getByRole("dialog", { name: "Объединить приложения" });
    await expect(dialog).toBeVisible();
    const combobox = dialog.getByRole("combobox", { name: "Приложение-источник" });
    await expect(combobox).toBeFocused();
    await expectNoA11yViolations(page);

    await combobox.click();
    await page.getByPlaceholder("Найти приложение…").fill(sourceName);
    await page.getByRole("option", { name: sourceName }).click();
    await expect(page.getByRole("listbox")).toBeHidden();
    await expect(combobox).toHaveText(sourceName);
    const preview = dialog.getByRole("region", { name: "Что переедет" });
    await expect(preview.getByRole("definition").first()).toHaveText("1");

    const submit = dialog.getByRole("button", { name: "Объединить", exact: true });
    const confirm = dialog.getByRole("textbox", {
      name: "Введите название источника, чтобы подтвердить",
    });
    await confirm.fill(sourceName.toLowerCase());
    await expect(submit).toBeDisabled();
    await confirm.fill(sourceName);
    await expect(submit).toBeEnabled();
    // Кнопка проявляется из opacity 0.5 (transition): axe — после окончания перехода.
    await expect(submit).toHaveCSS("opacity", "1");
    await expectNoA11yViolations(page);
    await submit.click();
    await expect(page.getByText("Объединение запущено")).toBeVisible();
    await expect(dialog).toBeHidden();
  });

  await test.step("banner shows progress, then the merge is done", async () => {
    // Worker берёт слияние в очередном запуске app_merge (раз в минуту) и с MERGE_KEYS_WAIT=0 проходит все шаги.
    const banner = page.getByRole("status").filter({ hasText: `Объединение с «${sourceName}»` });
    await expect(banner).toBeVisible();
    await expectNoA11yViolations(page);
    await expect(page.getByText("Приложения объединены")).toBeVisible({ timeout: 90_000 });
    await expect(banner).toBeHidden();
  });

  await test.step("the source is gone from the portfolio", async () => {
    expect(await appIdByName(page.request, sourceName)).toBeUndefined();
    await page.goto("/portfolio");
    const table = page.getByRole("table", { name: /Портфель приложений за/ });
    await expect(table.getByRole("link", { name: targetName, exact: true })).toBeVisible();
    await expect(table.getByRole("link", { name: sourceName, exact: true })).toHaveCount(0);
  });

  await test.step("the source key still works and lands in the target", async () => {
    // Ingest видит перенесённый ключ после перезагрузки кэша (KEY_CACHE_REFRESH): до неё событие уходит
    // по старому снимку в удалённый источник, поэтому сессии отправляются, пока одна не появится в цели.
    const sent: string[] = [];
    let landed: string | undefined;
    await expect
      .poll(
        async () => {
          sent.push(await sendSession(page.request, source.key("android")));
          const res = await page.request.get(`/api/v1/apps/${target.id}/realtime`);
          expect(res.ok()).toBe(true);
          const events = ((await res.json()) as Realtime).recent_events;
          landed = sent.find((id) =>
            events.some((e) => e.install_id === id && e.platform === "android"),
          );
          return landed;
        },
        { timeout: 60_000, intervals: [3_000] },
      )
      .toBeDefined();

    await page.goto(`/apps/${target.id}/realtime`);
    await expect(page.getByRole("heading", { level: 1, name: targetName })).toBeVisible();
    await expect(
      page
        .getByRole("table", { name: "Последние события" })
        .getByRole("link", { name: `Установка ${landed}` }),
    ).toBeVisible({ timeout: 15_000 });
    await expectNoA11yViolations(page);
  });
});
