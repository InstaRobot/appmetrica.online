import { ArrowRight, ChartColumn } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { Logo } from "@/components/layout/logo";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { cn } from "@/lib/utils";
import { HeroPreview } from "./hero-preview";

// S-00 Лендинг (макет «Dashboard v2», решение владельца от 2026-10-14): публичная страница на «/».
// Серверный компонент без данных: клиентский код — только переключатель темы.
export const GITHUB_URL = "https://github.com/InstaRobot/appmetrica.online";

// Маршрут /demo — route handler (включает демо-режим), typedRoutes его не перечисляет.
const DEMO = "/demo" as Route;

const FACTS = [
  { value: "≤ 500", label: "событий в батче" },
  { value: "7 дней", label: "окно офлайн-досыла" },
  { value: "25 мес", label: "хранение сырых событий" },
  { value: "≤ 500 мс", label: "P99 приёма событий" },
] as const;

const FEATURES = [
  {
    icon: "{ }",
    title: "Приём событий по HTTP",
    text: "POST /v1/events: батч до 500 событий, идемпотентность по insert_id, ответ 200 только после записи в ClickHouse.",
  },
  {
    icon: "Σ",
    title: "Портфель",
    text: "Все приложения на одном экране: DAU, новые, сессии, D1/D7 и тренд за 30 дней. Утренний дайджест в Telegram.",
  },
  {
    icon: "◔",
    title: "Аудитория",
    text: "DAU/WAU/MAU, новые, сессии и длительность. Разбивки по версии, ОС, устройству, стране и языку, сравнение периодов.",
  },
  {
    icon: "D7",
    title: "Retention",
    text: "Классические когорты D0–D30 и W0–W12 с определениями как в AppMetrica. Heatmap и средняя кривая.",
  },
  {
    icon: "●",
    title: "Realtime",
    text: "Активные сейчас, старты сессий по минутам и лента последних событий с переходом в профиль.",
  },
  {
    icon: "⌫",
    title: "GDPR и приватность",
    text: "IP не хранится — только страна и город. Удаление данных человека по запросу, журнал удалений, PII-guard для user_id.",
  },
] as const;

const STEPS = [
  "Создайте приложение: название, платформа, часовой пояс. Write-ключ pk_… выпускается автоматически.",
  "Отправьте первый батч с session_start — curl, Swift, Kotlin, JavaScript или Go.",
  "Статус «Ждём первое событие» сменится на «Получено», и приложение появится в портфеле.",
] as const;

// Адрес ingest — из сборки (NEXT_PUBLIC_INGEST_URL, у self-hosted установки свой домен); читается
// внутри функции, а не на уровне модуля (AGENTS.md §8).
const DEFAULT_INGEST_URL = "https://i.appmetrica.online";

const snippet = (ingestUrl: string) => `curl -X POST ${ingestUrl}/v1/events \\
  -H "Content-Type: application/json" \\
  -H "X-App-Key: $AM_PK" \\
  -d '{
    "sent_at": "2026-10-14T09:00:00Z",
    "context": { "platform": "ios" },
    "events": [{
      "insert_id": "4b1c…",
      "name": "session_start",
      "timestamp": "2026-10-14T09:00:00Z",
      "install_id": "5d8e…",
      "session_id": "a0f1…"
    }]
  }'

→ 200 {"accepted":1,"rejected":[],"warnings":[]}`;

const BIG_BUTTON =
  "inline-flex h-12 items-center gap-2.5 rounded-xl px-5.5 text-[15px] font-semibold transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50";
const PRIMARY =
  "bg-linear-to-b from-(--primary-top) to-primary text-primary-foreground shadow-(--shadow-primary) hover:from-(--primary-hover) hover:to-(--primary-hover)";
const OUTLINE = "border border-(--border-strong) bg-card text-foreground hover:bg-subtle";

export function LandingPage() {
  return (
    <div className="flex min-h-screen flex-col">
      <header className="mx-auto flex h-16 w-full max-w-[1200px] items-center gap-4 px-8">
        <Link href="/" aria-label="appmetrica.online — на главную" className="rounded-md">
          <Logo />
        </Link>
        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle className="border border-(--border-strong) bg-card" />
          <Link
            href="/login"
            className={cn(
              "inline-flex h-9 items-center rounded-lg px-4 text-sm font-semibold transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
              PRIMARY,
            )}
          >
            Войти
          </Link>
        </div>
      </header>

      <main className="mx-auto flex w-full max-w-[1200px] flex-1 flex-col gap-18 px-8 pt-12 pb-20">
        <section className="grid grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)] items-center gap-12">
          <div className="flex flex-col gap-6">
            <span className="inline-flex h-7 items-center gap-2 self-start rounded-full bg-selected px-3 text-xs font-semibold text-fg-link">
              <span aria-hidden="true" className="size-1.5 rounded-full bg-success-500" />
              Данные в ЕС · без SDK · без семплирования
            </span>
            <h1 className="text-[52px] leading-[56px] font-extrabold tracking-[-0.035em] text-balance">
              Аналитика ваших приложений — по одному HTTP-запросу
            </h1>
            <p className="max-w-[520px] text-lg leading-7 text-fg-muted text-pretty">
              Портфель всех приложений, аудитория, retention и realtime — считаются по людям, а не
              по устройствам. Одно событие{" "}
              <code className="font-mono text-base">session_start</code> — и приложение уже в
              отчётах.
            </p>
            <div className="flex flex-wrap gap-3">
              <Link href="/login" className={cn(BIG_BUTTON, PRIMARY)}>
                Войти в панель
                <ArrowRight aria-hidden="true" className="size-4" />
              </Link>
              <Link href={DEMO} prefetch={false} className={cn(BIG_BUTTON, OUTLINE)}>
                <ChartColumn aria-hidden="true" className="size-4" />
                Посмотреть демо-данные
              </Link>
              <a
                href={GITHUB_URL}
                target="_blank"
                rel="noopener noreferrer"
                className={cn(BIG_BUTTON, OUTLINE)}
              >
                <GitHubMark />
                GitHub
              </a>
            </div>
            <ul aria-label="Ключевые ограничения" className="flex flex-wrap gap-8 pt-2">
              {FACTS.map((f) => (
                <li key={f.label} className="flex flex-col gap-0.5">
                  <span className="text-2xl font-extrabold tracking-[-0.03em]">{f.value}</span>
                  <span className="text-caption text-fg-muted">{f.label}</span>
                </li>
              ))}
            </ul>
          </div>
          <HeroPreview />
        </section>

        <section aria-labelledby="landing-features" className="flex flex-col gap-6">
          <div className="flex max-w-[640px] flex-col gap-2">
            <h2
              id="landing-features"
              className="text-[32px] leading-[38px] font-extrabold tracking-[-0.03em]"
            >
              Что внутри панели
            </h2>
            <p className="text-base leading-6 text-fg-muted">
              Фиксированный набор отчётов, которые нужны каждое утро. Без конструктора и лишних
              настроек.
            </p>
          </div>
          <div className="grid grid-cols-3 gap-4">
            {FEATURES.map((f) => (
              <article
                key={f.title}
                className="flex flex-col gap-3 rounded-2xl border border-border bg-card p-6 shadow-(--shadow-card)"
              >
                <span
                  aria-hidden="true"
                  className="flex size-10 items-center justify-center rounded-xl bg-selected font-mono text-[13px] font-bold text-fg-link"
                >
                  {f.icon}
                </span>
                <h3 className="text-heading-3">{f.title}</h3>
                <p className="text-body-md text-fg-muted">{f.text}</p>
              </article>
            ))}
          </div>
        </section>

        <section
          aria-labelledby="landing-integration"
          className="grid grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)] items-center gap-12"
        >
          <div className="flex flex-col gap-6">
            <div className="flex flex-col gap-2">
              <h2
                id="landing-integration"
                className="text-[32px] leading-[38px] font-extrabold tracking-[-0.03em]"
              >
                Интеграция за пять минут
              </h2>
              <p className="text-base leading-6 text-fg-muted">
                Создайте приложение, скопируйте write-ключ и отправьте первый батч. Формат одинаков
                для iOS, Android, веба, ботов и бэкендов — SDK не нужен.
              </p>
            </div>
            <ol aria-label="Шаги интеграции" className="flex flex-col gap-3">
              {STEPS.map((text, i) => (
                <li key={text} className="flex items-start gap-3">
                  <span
                    aria-hidden="true"
                    className="flex size-7 shrink-0 items-center justify-center rounded-full bg-selected text-sm font-bold text-fg-link"
                  >
                    {i + 1}
                  </span>
                  <span className="pt-1 text-body-md">{text}</span>
                </li>
              ))}
            </ol>
          </div>
          <figure aria-label="Пример запроса на приём событий" className="min-w-0">
            <pre className="overflow-x-auto rounded-2xl bg-code-bg p-6 font-mono text-[13px] leading-5 text-code-fg shadow-(--shadow-card)">
              {snippet(process.env.NEXT_PUBLIC_INGEST_URL || DEFAULT_INGEST_URL)}
            </pre>
          </figure>
        </section>

        <section
          aria-labelledby="landing-admin"
          className="flex items-center justify-between gap-8 rounded-2xl bg-linear-135 from-(--primary-top) to-(--primary-hover) p-8 text-primary-foreground shadow-(--shadow-primary)"
        >
          <div className="flex flex-col gap-2">
            <h2 id="landing-admin" className="text-2xl font-extrabold tracking-[-0.02em]">
              Панель администратора
            </h2>
            <p className="max-w-[640px] text-base leading-6">
              Вход только по passkey. Приватные данные пользователей не покидают ЕС.
            </p>
          </div>
          <Link
            href="/login"
            className="inline-flex h-11 shrink-0 items-center rounded-xl bg-white px-5 text-[15px] font-semibold text-[#2d55c7] outline-none hover:bg-white/90 focus-visible:ring-3 focus-visible:ring-white/60"
          >
            Войти
          </Link>
        </section>
      </main>

      <footer className="mx-auto flex w-full max-w-[1200px] flex-wrap items-center gap-2 px-8 pb-8 text-caption text-fg-muted">
        <span>© 2026 appmetrica.online</span>
        <span aria-hidden="true">·</span>
        <span>
          <a href="https://db-ip.com" className="text-fg-link hover:underline">
            IP Geolocation by DB-IP
          </a>{" "}
          (CC BY 4.0)
        </span>
        <span aria-hidden="true">·</span>
        <span>Hetzner FSN1, ЕС</span>
      </footer>
    </div>
  );
}

function GitHubMark() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className="size-[18px]" fill="currentColor">
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.1.79-.25.79-.56v-1.96c-3.2.7-3.87-1.54-3.87-1.54-.52-1.33-1.28-1.69-1.28-1.69-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.18 1.76 1.18 1.03 1.76 2.69 1.25 3.35.96.1-.75.4-1.25.73-1.54-2.55-.29-5.24-1.28-5.24-5.68 0-1.25.45-2.28 1.18-3.08-.12-.29-.51-1.46.11-3.04 0 0 .96-.31 3.15 1.18a10.9 10.9 0 0 1 5.74 0c2.19-1.49 3.15-1.18 3.15-1.18.62 1.58.23 2.75.11 3.04.74.8 1.18 1.83 1.18 3.08 0 4.41-2.69 5.39-5.25 5.67.41.36.78 1.06.78 2.14v3.17c0 .31.21.67.8.56A11.5 11.5 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5Z" />
    </svg>
  );
}
