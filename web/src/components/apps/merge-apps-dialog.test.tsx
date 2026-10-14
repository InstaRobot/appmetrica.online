import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { appMerge, appSummary, errorBody, app as makeApp, mergePreview } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { MergeAppsDialog } from "./merge-apps-dialog";

const TARGET_ID = "0192aaaa-0000-7000-8000-000000000017";
const SOURCE_ID = "0192aaaa-0000-7000-8000-000000000018";
const ARCHIVED_ID = "0192aaaa-0000-7000-8000-000000000019";

const TARGET = makeApp({ id: TARGET_ID, name: "Notes iOS", timezone: "Europe/Moscow" });
const SOURCE = makeApp({
  id: SOURCE_ID,
  name: "Notes Android",
  platform: "android",
  platforms: ["android"],
});
const ARCHIVED = makeApp({
  id: ARCHIVED_ID,
  name: "Old Game",
  status: "archived",
  archived_at: "2026-09-01T10:00:00Z",
});

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

type MockOptions = {
  preview?: (n: number) => Response;
  create?: () => Response;
};

function mockApi(opts: MockOptions = {}) {
  const calls = { apps: [] as URLSearchParams[], preview: [] as string[], create: [] as unknown[] };
  server.use(
    http.get("*/v1/apps", ({ request }) => {
      calls.apps.push(new URL(request.url).searchParams);
      return HttpResponse.json({ items: [TARGET, SOURCE, ARCHIVED] });
    }),
    http.get(`*/admin/v1/apps/${TARGET_ID}/merge-preview`, ({ request }) => {
      const n = calls.preview.length;
      calls.preview.push(new URL(request.url).searchParams.get("source_app_id") ?? "");
      if (opts.preview) return opts.preview(n);
      return HttpResponse.json(
        mergePreview({
          source: appSummary({ id: SOURCE_ID, name: "Notes Android", platforms: ["android"] }),
        }),
      );
    }),
    http.post(`*/admin/v1/apps/${TARGET_ID}/merges`, async ({ request }) => {
      calls.create.push(await request.json());
      if (opts.create) return opts.create();
      return HttpResponse.json(appMerge({ target_app_id: TARGET_ID, source_app_id: SOURCE_ID }), {
        status: 202,
      });
    }),
  );
  return calls;
}

async function chooseSource(user: ReturnType<typeof userEvent.setup>, name: RegExp) {
  await user.click(await screen.findByRole("combobox", { name: "Приложение-источник" }));
  await user.click(await screen.findByRole("option", { name }));
}

function renderDialog(onOpenChange = vi.fn()) {
  renderWithClient(<MergeAppsDialog target={TARGET} open onOpenChange={onOpenChange} />);
  return onOpenChange;
}

describe("MergeAppsDialog", () => {
  it("confirms by source name", async () => {
    const calls = mockApi();
    const onOpenChange = renderDialog();
    const user = userEvent.setup();

    const dialog = screen.getByRole("dialog", { name: "Объединить приложения" });
    expect(dialog).toHaveTextContent("Данные выбранного приложения переедут в «Notes iOS».");
    await chooseSource(user, /Notes Android/);

    const preview = await within(dialog).findByRole("region", { name: "Что переедет" });
    await waitFor(() => expect(preview).toHaveTextContent(/1\s204\s331/));
    expect(preview).toHaveAttribute("aria-live", "polite");
    expect(preview).toHaveTextContent("Ключей записи2");
    expect(preview).toHaveTextContent(
      "Ключи источника продолжат работать — выпущенные версии приложений обновлять не нужно.",
    );
    expect(calls.preview).toEqual([SOURCE_ID]);

    const submit = within(dialog).getByRole("button", { name: "Объединить" });
    const confirm = within(dialog).getByRole("textbox", {
      name: "Введите название источника, чтобы подтвердить",
    });
    await user.type(confirm, "notes android");
    expect(submit).toBeDisabled();
    await user.clear(confirm);
    await user.type(confirm, "Notes Android");
    expect(submit).toBeEnabled();
    await user.click(submit);

    await waitFor(() =>
      expect(calls.create).toEqual([{ source_app_id: SOURCE_ID, confirm_name: "Notes Android" }]),
    );
    await waitFor(() => expect(toasts.success).toHaveBeenCalledWith("Объединение запущено"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("shows a conflict", async () => {
    mockApi({
      create: () => HttpResponse.json(errorBody("merge_in_progress"), { status: 409 }),
    });
    const onOpenChange = renderDialog();
    const user = userEvent.setup();

    const dialog = screen.getByRole("dialog", { name: "Объединить приложения" });
    await chooseSource(user, /Notes Android/);
    await within(dialog).findByText("Ключей записи");
    await user.type(
      within(dialog).getByRole("textbox", {
        name: "Введите название источника, чтобы подтвердить",
      }),
      "Notes Android",
    );
    await user.click(within(dialog).getByRole("button", { name: "Объединить" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Приложение участвует в объединении — дождитесь завершения",
    );
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(toasts.success).not.toHaveBeenCalled();
  });

  it("offers other apps, archived included, but not the target", async () => {
    const calls = mockApi();
    renderDialog();
    const user = userEvent.setup();

    const combobox = await screen.findByRole("combobox", { name: "Приложение-источник" });
    await waitFor(() => expect(combobox).toHaveFocus());
    await user.click(combobox);
    const options = await screen.findAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["Notes Android", "Old Gameв архиве"]);
    expect(calls.apps[0]?.get("include_archived")).toBe("true");

    await user.type(screen.getByPlaceholderText("Найти приложение…"), "old");
    expect(screen.getAllByRole("option").map((o) => o.textContent)).toEqual(["Old Gameв архиве"]);
  });

  it("warns that the source days are recalculated in the target time zone", async () => {
    mockApi({
      preview: () =>
        HttpResponse.json(
          mergePreview({
            timezone_differs: true,
            source_timezone: "Asia/Almaty",
            first_day: null,
            last_day: null,
            events: 0,
          }),
        ),
    });
    renderDialog();
    const user = userEvent.setup();

    await chooseSource(user, /Notes Android/);
    const preview = await screen.findByRole("region", { name: "Что переедет" });
    await waitFor(() =>
      expect(preview).toHaveTextContent(
        "Часовой пояс останется «Europe/Moscow»: дни источника (Asia/Almaty) пересчитаются.",
      ),
    );
    expect(preview).toHaveTextContent("Дни данныхнет событий");
  });

  it("retries the preview when storage is unavailable", async () => {
    const calls = mockApi({
      preview: (n) =>
        n === 0
          ? HttpResponse.json(errorBody("storage_unavailable"), { status: 503 })
          : HttpResponse.json(mergePreview()),
    });
    renderDialog();
    const user = userEvent.setup();

    await chooseSource(user, /Notes Android/);
    const dialog = screen.getByRole("dialog", { name: "Объединить приложения" });
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Хранилище данных временно недоступно",
    );
    await user.type(
      within(dialog).getByRole("textbox", {
        name: "Введите название источника, чтобы подтвердить",
      }),
      "Notes Android",
    );
    expect(within(dialog).getByRole("button", { name: "Объединить" })).toBeDisabled();

    await user.click(within(dialog).getByRole("button", { name: "Повторить" }));
    await within(dialog).findByText("Ключей записи");
    expect(calls.preview).toHaveLength(2);
    expect(within(dialog).getByRole("button", { name: "Объединить" })).toBeEnabled();
  });

  it("shows a name mismatch from the server under the field", async () => {
    mockApi({
      create: () => HttpResponse.json(errorBody("confirm_name_mismatch"), { status: 409 }),
    });
    renderDialog();
    const user = userEvent.setup();

    const dialog = screen.getByRole("dialog", { name: "Объединить приложения" });
    await chooseSource(user, /Notes Android/);
    await within(dialog).findByText("Ключей записи");
    const confirm = within(dialog).getByRole("textbox", {
      name: "Введите название источника, чтобы подтвердить",
    });
    await user.type(confirm, "Notes Android");
    await user.click(within(dialog).getByRole("button", { name: "Объединить" }));

    await waitFor(() => expect(confirm).toHaveAccessibleDescription("Название не совпадает"));
    expect(confirm).toHaveAttribute("aria-invalid", "true");
  });

  it("explains that a source pending deletion cannot be merged", async () => {
    mockApi({
      create: () => HttpResponse.json(errorBody("deletion_pending"), { status: 409 }),
    });
    renderDialog();
    const user = userEvent.setup();

    const dialog = screen.getByRole("dialog", { name: "Объединить приложения" });
    await chooseSource(user, /Notes Android/);
    await within(dialog).findByText("Ключей записи");
    await user.type(
      within(dialog).getByRole("textbox", {
        name: "Введите название источника, чтобы подтвердить",
      }),
      "Notes Android",
    );
    await user.click(within(dialog).getByRole("button", { name: "Объединить" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Источник ожидает удаления — объединить нельзя",
    );
  });

  it("closes on cancel without creating a merge", async () => {
    const calls = mockApi();
    const onOpenChange = renderDialog();

    await userEvent.setup().click(screen.getByRole("button", { name: "Отмена" }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(calls.create).toEqual([]);
  });
});
