import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { generateMetadata } from "@/app/[lang]/page";
import { renderWithLocale } from "@/test/rtl";
import { GITHUB_URL, LandingPage } from "./landing-page";

// S-00 Лендинг (макет «Dashboard v2», решение владельца): публичная страница на «/».
describe("LandingPage", () => {
  it("leads with the product promise and the three entry points", () => {
    render(<LandingPage locale="ru" />);

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
    render(<LandingPage locale="ru" />);

    const header = screen.getByRole("banner");
    expect(within(header).getByRole("link", { name: "Документация" })).toHaveAttribute(
      "href",
      "/ru/docs",
    );
    expect(within(header).getByRole("link", { name: "Войти" })).toHaveAttribute("href", "/login");
    expect(within(header).getByRole("button", { name: /тему$/ })).toBeInTheDocument();
  });

  it("states the ingest limits from the spec", () => {
    render(<LandingPage locale="ru" />);

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
    render(<LandingPage locale="ru" />);

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
    render(<LandingPage locale="ru" />);

    const snippet = screen.getByLabelText("Пример запроса на приём событий");
    expect(snippet).toHaveTextContent("https://i.analytics.example.com/v1/events");
    expect(snippet).toHaveTextContent("X-App-Key: $AM_PK");
    expect(snippet.textContent).not.toMatch(/pk_[A-Za-z0-9]{8,}/);
  });

  it("keeps the example domain in the snippet whatever the installation's ingest URL", () => {
    vi.stubEnv("NEXT_PUBLIC_INGEST_URL", "https://i.appmetrica.online");
    render(<LandingPage locale="ru" />);

    const snippet = screen.getByLabelText("Пример запроса на приём событий");
    expect(snippet).toHaveTextContent("https://i.analytics.example.com/v1/events");
    expect(snippet).not.toHaveTextContent("i.appmetrica.online");
    vi.unstubAllEnvs();
  });

  it("promises passkey-only sign-in and keeps the DB-IP attribution", () => {
    render(<LandingPage locale="ru" />);

    // Без обещаний о стране хранения: где стоит сервер, решает установка.
    expect(screen.getByText("Вход только по passkey.")).toBeInTheDocument();
    expect(screen.queryByText(/учётной записи/)).not.toBeInTheDocument();
    const footer = screen.getByRole("contentinfo");
    expect(within(footer).getByRole("link", { name: "IP Geolocation by DB-IP" })).toHaveAttribute(
      "href",
      "https://db-ip.com",
    );
  });

  it("shows the Russian numbers in the dashboard preview", () => {
    render(<LandingPage locale="ru" />);

    const preview = screen.getByRole("figure", {
      name: "Пример отчёта: приложение Notes iOS за последние 30 дней",
    });
    // toHaveTextContent сводит неразрывные пробелы форматтера к обычным.
    expect(preview).toHaveTextContent("12 480");
    expect(preview).toHaveTextContent("▲ +4,1 %");
  });
});

// Телефон (< md): одна колонка, кнопки героя на всю ширину и не ниже 44 px, пример кода прокручивается
// внутри своего блока. Десктопные классы (lg:, xl:) сохраняют прежнюю сетку.
describe("LandingPage on a phone", () => {
  it("stacks the hero in one column and stretches its buttons", () => {
    render(<LandingPage locale="ru" />);

    const hero = screen.getByRole("heading", { level: 1 }).closest("section");
    expect(hero).toHaveClass("grid-cols-1");
    expect(hero?.className).toMatch(/\blg:grid-cols-\[/);

    const main = screen.getByRole("main");
    for (const name of ["Войти в панель", "Посмотреть демо-данные", "GitHub"]) {
      // h-12 — 48 px: цель нажатия не меньше 44 px.
      expect(within(main).getByRole("link", { name })).toHaveClass("h-12", "w-full", "md:w-auto");
    }
  });

  it("lays the report areas out in one, two, then three columns", () => {
    render(<LandingPage locale="ru" />);

    const grid = screen
      .getByRole("heading", { level: 3, name: "Портфель" })
      .closest("article")?.parentElement;
    expect(grid).toHaveClass("grid-cols-1", "md:grid-cols-2", "xl:grid-cols-3");
  });

  it("scrolls the curl snippet inside its own box", () => {
    render(<LandingPage locale="ru" />);

    const snippet = screen.getByLabelText("Пример запроса на приём событий");
    expect(snippet).toHaveClass("min-w-0");
    const pre = snippet.querySelector("pre");
    expect(pre).toHaveClass("overflow-x-auto");
    // Прокручиваемая область доступна с клавиатуры.
    expect(pre).toHaveAttribute("tabindex", "0");
  });

  it("puts the admin call to action under its text with a full-width button", () => {
    render(<LandingPage locale="ru" />);

    const admin = screen.getByRole("region", { name: "Панель администратора" });
    expect(admin).toHaveClass("flex-col", "md:flex-row");
    expect(within(admin).getByRole("link", { name: "Войти" })).toHaveClass("w-full", "md:w-auto");
  });
});

describe("LandingPage in English", () => {
  const renderEnglish = () => renderWithLocale(<LandingPage locale="en" />, "en");

  it("leads with the product promise and the English entry points", () => {
    renderEnglish();

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "Analytics for your apps — with a single HTTP request",
      }),
    ).toBeInTheDocument();
    expect(screen.getByText(/counted by people, not devices/)).toHaveTextContent(
      "One session_start event, and the app is already in your reports.",
    );
    const main = screen.getByRole("main");
    expect(within(main).getByRole("link", { name: "Sign in to the dashboard" })).toHaveAttribute(
      "href",
      "/login",
    );
    expect(within(main).getByRole("link", { name: "View demo data" })).toHaveAttribute(
      "href",
      "/demo",
    );
    const header = screen.getByRole("banner");
    expect(within(header).getByRole("link", { name: "Documentation" })).toHaveAttribute(
      "href",
      "/docs",
    );
    expect(within(header).getByRole("link", { name: "appmetrica.online — home" })).toHaveAttribute(
      "href",
      "/",
    );
  });

  it("states the limits, the report areas and the steps in English", () => {
    renderEnglish();

    expect(
      within(screen.getByRole("list", { name: "Key limits" }))
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual([
      "≤ 500events per batch",
      "7 daysoffline resend window",
      "25 moraw event retention",
      "≤ 500 msP99 ingest latency",
    ]);
    expect(screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual([
      "HTTP event ingestion",
      "Portfolio",
      "Audience",
      "Retention",
      "Realtime",
      "GDPR and privacy",
    ]);
    expect(
      within(screen.getByRole("list", { name: "Integration steps" })).getAllByRole("listitem"),
    ).toHaveLength(3);
    expect(screen.getByLabelText("Sample event ingestion request")).toHaveTextContent(
      "X-App-Key: $AM_PK",
    );
  });

  it("shows the preview in English formats and no hosting in the footer", () => {
    renderEnglish();

    const preview = screen.getByRole("figure", {
      name: "Sample report: the Notes iOS app over the last 30 days",
    });
    expect(preview).toHaveTextContent("Last 30 days");
    expect(preview).toHaveTextContent("12,480");
    expect(preview).toHaveTextContent("▲ +4.1%");
    expect(preview).toHaveTextContent("Daily active users");
    expect(screen.getByText("Passkey-only sign-in.")).toBeInTheDocument();
    // Подвал — копирайт и атрибуция DB-IP (CC BY), без названия хостинга.
    expect(screen.getByRole("contentinfo")).not.toHaveTextContent(/Hetzner|FSN1/);
  });
});

const metadataOf = (lang: string) =>
  generateMetadata({ params: Promise.resolve({ lang }), searchParams: Promise.resolve({}) });

// Метаданные лендинга (app/[lang]/page.tsx) — на языке адреса: «/» — английские, «/ru» — русские.
describe("landing metadata", () => {
  it("speaks the language of the address", async () => {
    expect(await metadataOf("en")).toEqual({
      title: { absolute: "appmetrica.online — app analytics without an SDK" },
      description: "App analytics without an SDK",
    });
    expect(await metadataOf("ru")).toEqual({
      title: { absolute: "appmetrica.online — аналитика приложений без SDK" },
      description: "Аналитика приложений без SDK",
    });
  });
});
