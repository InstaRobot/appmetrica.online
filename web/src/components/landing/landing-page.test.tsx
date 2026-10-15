import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { GITHUB_URL, LandingPage } from "./landing-page";

// S-00 Лендинг (макет «Dashboard v2», решение владельца): публичная страница на «/».
describe("LandingPage", () => {
  it("leads with the product promise and the three entry points", () => {
    render(<LandingPage />);

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "Аналитика ваших приложений — по одному HTTP-запросу",
      }),
    ).toBeInTheDocument();

    const main = screen.getByRole("main");
    expect(within(main).getByRole("link", { name: "Войти в панель" })).toHaveAttribute(
      "href",
      "/login",
    );
    expect(within(main).getByRole("link", { name: "Посмотреть демо-данные" })).toHaveAttribute(
      "href",
      "/demo",
    );
    const github = within(main).getByRole("link", { name: "GitHub" });
    expect(github).toHaveAttribute("href", "https://github.com/InstaRobot/appmetrica.online");
    expect(github).toHaveAttribute("target", "_blank");
    expect(github.getAttribute("rel")).toContain("noopener");
    expect(GITHUB_URL).toBe("https://github.com/InstaRobot/appmetrica.online");
  });

  it("has its own header with the logo, docs, theme switch and sign-in", () => {
    render(<LandingPage />);

    const header = screen.getByRole("banner");
    expect(within(header).getByRole("link", { name: "Документация" })).toHaveAttribute(
      "href",
      "/ru/docs",
    );
    expect(within(header).getByRole("link", { name: "Войти" })).toHaveAttribute("href", "/login");
    expect(within(header).getByRole("button", { name: /тему$/ })).toBeInTheDocument();
  });

  it("states the ingest limits from the spec", () => {
    render(<LandingPage />);

    const facts = screen.getByRole("list", { name: "Ключевые ограничения" });
    expect(
      within(facts)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual([
      "≤ 500событий в батче",
      "7 днейокно офлайн-досыла",
      "25 месхранение сырых событий",
      "≤ 500 мсP99 приёма событий",
    ]);
  });

  it("lists the six report areas and the three integration steps", () => {
    render(<LandingPage />);

    expect(screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual([
      "Приём событий по HTTP",
      "Портфель",
      "Аудитория",
      "Retention",
      "Realtime",
      "GDPR и приватность",
    ]);
    expect(
      within(screen.getByRole("list", { name: "Шаги интеграции" })).getAllByRole("listitem"),
    ).toHaveLength(3);
  });

  it("shows a curl snippet without a real write key", () => {
    render(<LandingPage />);

    const snippet = screen.getByLabelText("Пример запроса на приём событий");
    expect(snippet).toHaveTextContent("https://i.analytics.example.com/v1/events");
    expect(snippet).toHaveTextContent("X-App-Key: $AM_PK");
    expect(snippet.textContent).not.toMatch(/pk_[A-Za-z0-9]{8,}/);
  });

  it("keeps the example domain in the snippet whatever the installation's ingest URL", () => {
    vi.stubEnv("NEXT_PUBLIC_INGEST_URL", "https://i.appmetrica.online");
    render(<LandingPage />);

    const snippet = screen.getByLabelText("Пример запроса на приём событий");
    expect(snippet).toHaveTextContent("https://i.analytics.example.com/v1/events");
    expect(snippet).not.toHaveTextContent("i.appmetrica.online");
    vi.unstubAllEnvs();
  });

  it("promises passkey-only sign-in and keeps the DB-IP attribution", () => {
    render(<LandingPage />);

    expect(screen.getByText(/Вход только по passkey/)).toBeInTheDocument();
    expect(screen.queryByText(/учётной записи/)).not.toBeInTheDocument();
    const footer = screen.getByRole("contentinfo");
    expect(within(footer).getByRole("link", { name: "IP Geolocation by DB-IP" })).toHaveAttribute(
      "href",
      "https://db-ip.com",
    );
  });
});
