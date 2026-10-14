import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { type OnUrlUpdateFunction, withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiKey, passkey, settings, systemStatus } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { SettingsView } from "./settings-view";

const TABS = ["Passkeys", "API-ключи", "Telegram", "Система", "Удаления"];

function renderView(searchParams = "", onUrlUpdate: OnUrlUpdateFunction = vi.fn()) {
  return renderWithClient(<SettingsView />, {
    wrapper: withNuqsTestingAdapter({ searchParams, onUrlUpdate }),
  });
}

beforeEach(() => {
  server.use(
    http.get("*/admin/v1/passkeys", () => HttpResponse.json({ items: [passkey()] })),
    http.get("*/admin/v1/api-keys", () => HttpResponse.json({ items: [apiKey()] })),
    http.get("*/admin/v1/settings", () => HttpResponse.json(settings())),
    http.get("*/admin/v1/system/status", () => HttpResponse.json(systemStatus())),
    http.get("*/admin/v1/deletion-requests", () => HttpResponse.json({ items: [] })),
    http.get("*/v1/apps", () => HttpResponse.json({ items: [] })),
  );
});

describe("SettingsView", () => {
  it("tabs are keyboard navigable", async () => {
    renderView();
    const user = userEvent.setup();

    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(TABS);
    expect(screen.getByRole("tablist", { name: "Разделы настроек" })).toBeInTheDocument();
    expect(tabs[0]).toHaveAttribute("aria-selected", "true");

    await user.tab();
    expect(tabs[0]).toHaveFocus();
    for (let i = 1; i < TABS.length; i++) {
      await user.keyboard("{ArrowRight}");
      expect(tabs[i]).toHaveFocus();
      expect(tabs[i]).toHaveAttribute("aria-selected", "true");
      expect(tabs[i - 1]).toHaveAttribute("aria-selected", "false");
    }
    // Стрелка с последней вкладки возвращает на первую, влево — обратно на последнюю.
    await user.keyboard("{ArrowRight}");
    expect(tabs[0]).toHaveFocus();
    expect(tabs[0]).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{ArrowLeft}");
    expect(tabs[4]).toHaveFocus();
    expect(tabs[4]).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{Home}");
    expect(tabs[0]).toHaveFocus();
  });

  it("opens the tab from ?tab=", async () => {
    renderView("?tab=telegram");

    expect(screen.getByRole("tab", { name: "Telegram" })).toHaveAttribute("aria-selected", "true");
    expect(await screen.findByRole("textbox", { name: "Chat ID" })).toHaveValue("123456789");
  });

  it("falls back to passkeys for an unknown tab", async () => {
    renderView("?tab=nope");

    expect(screen.getByRole("tab", { name: "Passkeys" })).toHaveAttribute("aria-selected", "true");
    expect(await screen.findByRole("table", { name: "Passkeys" })).toBeInTheDocument();
  });

  it("writes the selected tab to the URL", async () => {
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    renderView("", onUrlUpdate);

    await userEvent.setup().click(screen.getByRole("tab", { name: "Система" }));
    expect(onUrlUpdate).toHaveBeenLastCalledWith(
      expect.objectContaining({ queryString: "?tab=system" }),
    );
    expect(await screen.findByText(/ClickHouse/)).toBeInTheDocument();
  });
});
