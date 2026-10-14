import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { appMerge, errorBody, app as makeApp } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { AppShell } from "./app-shell";

const APP_ID = "0192aaaa-0000-7000-8000-000000000001";

vi.mock("next/navigation", () => ({
  useParams: () => ({ appId: APP_ID }),
  usePathname: () => `/apps/${APP_ID}`,
  useSearchParams: () => new URLSearchParams("period=30d&metric=new"),
}));

beforeEach(() => {
  server.use(http.get(`*/admin/v1/apps/${APP_ID}/merges`, () => HttpResponse.json({ items: [] })));
});

function renderShell(searchParams = "") {
  return renderWithClient(
    <AppShell>
      <p>Отчёт</p>
    </AppShell>,
    { wrapper: withNuqsTestingAdapter({ searchParams }) },
  );
}

describe("AppShell", () => {
  it("shows the app name, platform, tabs and the report", async () => {
    server.use(
      http.get(`*/v1/apps/${APP_ID}`, () =>
        HttpResponse.json(makeApp({ id: APP_ID, name: "Notes iOS", platform: "ios" })),
      ),
    );
    renderShell("?unit=installs");

    expect(await screen.findByRole("heading", { level: 1, name: "Notes iOS" })).toBeInTheDocument();
    expect(screen.getByText("iOS", { selector: "[data-slot=badge]" })).toBeInTheDocument();
    expect(screen.getByText("Установки")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Аудитория" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Retention" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/retention?period=30d`,
    );
    expect(screen.getByText("Отчёт")).toBeInTheDocument();
    expect(screen.queryByText(/Приложение в архиве/)).not.toBeInTheDocument();
  });

  it("names every platform of the app in the header", async () => {
    server.use(
      http.get(`*/v1/apps/${APP_ID}`, () =>
        HttpResponse.json(
          makeApp({
            id: APP_ID,
            name: "Notes",
            platform: "ios",
            platforms: ["ios", "android", "web", "macos"],
          }),
        ),
      ),
    );
    renderShell();

    expect(await screen.findByRole("heading", { level: 1, name: "Notes" })).toBeInTheDocument();
    const badge = screen.getByText("iOS, Android, Веб, macOS", { selector: "[data-slot=badge]" });
    expect(badge.querySelectorAll("svg")).toHaveLength(4);
  });

  it("warns about an archived app", async () => {
    server.use(
      http.get(`*/v1/apps/${APP_ID}`, () =>
        HttpResponse.json(
          makeApp({ id: APP_ID, status: "archived", archived_at: "2026-10-01T00:00:00Z" }),
        ),
      ),
    );
    renderShell();
    expect(await screen.findByText(/Приложение в архиве/)).toBeInTheDocument();
  });

  it("shows the progress of a merge into the app", async () => {
    server.use(
      http.get(`*/v1/apps/${APP_ID}`, () => HttpResponse.json(makeApp({ id: APP_ID }))),
      http.get(`*/admin/v1/apps/${APP_ID}/merges`, () =>
        HttpResponse.json({
          items: [appMerge({ target_app_id: APP_ID, status: "running", step: "wait_keys" })],
        }),
      ),
    );
    renderShell();
    expect(await screen.findByText(/^Объединение с «Notes Android»/)).toHaveTextContent(
      "Объединение с «Notes Android»: ждём, пока приём событий перечитает ключи (до 17 мин)",
    );
    expect(screen.getByText("Отчёт")).toBeInTheDocument();
  });

  it("shows not found instead of the report for an unknown app", async () => {
    server.use(
      http.get(`*/v1/apps/${APP_ID}`, () =>
        HttpResponse.json(errorBody("app_not_found"), { status: 404 }),
      ),
    );
    renderShell();

    expect(
      await screen.findByRole("heading", { name: "Приложение не найдено или удалено" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "В портфель" })).toHaveAttribute("href", "/portfolio");
    expect(screen.queryByText("Отчёт")).not.toBeInTheDocument();
  });

  it("shows a retryable error when the app cannot be loaded", async () => {
    server.use(
      http.get(`*/v1/apps/${APP_ID}`, () =>
        HttpResponse.json(errorBody("internal_error"), { status: 500 }),
      ),
    );
    renderShell();
    expect(await screen.findByRole("button", { name: "Повторить" })).toBeInTheDocument();
    expect(screen.queryByText("Отчёт")).not.toBeInTheDocument();
  });
});
