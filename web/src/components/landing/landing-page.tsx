import { ArrowRight, ChartColumn } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { richText } from "@/components/common/rich-text";
import { PublicHeader } from "@/components/layout/public-header";
import type { Locale } from "@/lib/i18n/locale";
import { cn } from "@/lib/utils";
import { getMessages, type Messages } from "@/messages";
import { HeroPreview } from "./hero-preview";

// S-00 Лендинг (макет «Dashboard v2», решение владельца от 2026-10-14): публичная страница на «/» и «/ru».
// Серверный компонент без данных: язык приходит из адреса, клиентский код — только переключатели темы и
// языка в шапке.
export const GITHUB_URL = "https://github.com/InstaRobot/appmetrica.online";

// Маршрут /demo — route handler (включает демо-режим), typedRoutes его не перечисляет.
const DEMO = "/demo" as Route;

type LandingMessages = Messages["landing"];

const FACTS = ["batch", "offline", "storage", "latency"] as const satisfies readonly Exclude<
  keyof LandingMessages["facts"],
  "label"
>[];

const FEATURES = [
  { key: "ingest", icon: "{ }" },
  { key: "portfolio", icon: "Σ" },
  { key: "audience", icon: "◔" },
  { key: "retention", icon: "D7" },
  { key: "realtime", icon: "●" },
  { key: "privacy", icon: "⌫" },
] as const satisfies readonly {
  key: Exclude<keyof LandingMessages["features"], "title" | "lead">;
  icon: string;
}[];

const STEPS = [
  "create",
  "send",
  "done",
] as const satisfies readonly (keyof LandingMessages["integration"]["steps"])[];

// Пример на лендинге — с условным доменом (analytics.example.com, как в README), а не с адресом этой
// установки: лендинг показывает формат запроса, свой адрес ingest владелец видит во вкладке «Интеграция».
const SNIPPET = `curl -X POST https://i.analytics.example.com/v1/events \\
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

// Телефон: кнопки героя на всю ширину (h-12 — 48 px, цель нажатия не меньше 44 px), с md — по содержимому.
const BIG_BUTTON =
  "inline-flex h-12 w-full items-center justify-center gap-2.5 rounded-xl px-5.5 text-[15px] font-semibold transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50 md:w-auto";
const PRIMARY =
  "bg-linear-to-b from-(--primary-top) to-primary text-primary-foreground shadow-(--shadow-primary) hover:from-(--primary-hover) hover:to-(--primary-hover)";
const OUTLINE = "border border-(--border-strong) bg-card text-foreground hover:bg-subtle";
const H2 =
  "text-[26px] leading-[32px] font-extrabold tracking-[-0.03em] md:text-[32px] md:leading-[38px]";

export function LandingPage({ locale }: { locale: Locale }) {
  const m = getMessages(locale).landing;
  return (
    <div className="flex min-h-screen flex-col">
      <PublicHeader className="mx-auto max-w-[1200px]" />

      {/* Mobile-first: одна колонка на телефоне и планшете, две — с lg; десктопные отступы — с md. */}
      <main className="mx-auto flex w-full max-w-[1200px] flex-1 flex-col gap-12 px-4 pt-8 pb-12 md:gap-18 md:px-8 md:pt-12 md:pb-20">
        <section className="grid grid-cols-1 items-center gap-10 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)] lg:gap-12">
          <div className="flex flex-col gap-6">
            <span className="inline-flex min-h-7 items-center gap-2 self-start rounded-full bg-selected px-3 py-1 text-xs font-semibold text-fg-link">
              <span aria-hidden="true" className="size-1.5 rounded-full bg-success-500" />
              {m.hero.badge}
            </span>
            <h1 className="text-[36px] leading-[40px] font-extrabold tracking-[-0.035em] text-balance md:text-[44px] md:leading-[48px] lg:text-[52px] lg:leading-[56px]">
              {m.hero.title}
            </h1>
            <p className="max-w-[520px] text-base leading-6 text-fg-muted text-pretty md:text-lg md:leading-7">
              {richText(m.hero.lead, {
                event: <code className="font-mono text-base">session_start</code>,
              })}
            </p>
            <div className="flex flex-col gap-3 md:flex-row md:flex-wrap">
              <Link href="/login" className={cn(BIG_BUTTON, PRIMARY)}>
                {m.hero.signIn}
                <ArrowRight aria-hidden="true" className="size-4" />
              </Link>
              <Link href={DEMO} prefetch={false} className={cn(BIG_BUTTON, OUTLINE)}>
                <ChartColumn aria-hidden="true" className="size-4" />
                {m.hero.demo}
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
            <ul
              aria-label={m.facts.label}
              className="grid grid-cols-2 gap-x-6 gap-y-4 pt-2 md:flex md:flex-wrap md:gap-8"
            >
              {FACTS.map((key) => (
                <li key={key} className="flex flex-col gap-0.5">
                  <span className="text-2xl font-extrabold tracking-[-0.03em]">
                    {m.facts[key].value}
                  </span>
                  <span className="text-caption text-fg-muted">{m.facts[key].label}</span>
                </li>
              ))}
            </ul>
          </div>
          <HeroPreview locale={locale} />
        </section>

        <section aria-labelledby="landing-features" className="flex flex-col gap-6">
          <div className="flex max-w-[640px] flex-col gap-2">
            <h2 id="landing-features" className={H2}>
              {m.features.title}
            </h2>
            <p className="text-base leading-6 text-fg-muted">{m.features.lead}</p>
          </div>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {FEATURES.map((f) => (
              <article
                key={f.key}
                className="flex flex-col gap-3 rounded-2xl border border-border bg-card p-5 shadow-(--shadow-card) md:p-6"
              >
                <span
                  aria-hidden="true"
                  className="flex size-10 items-center justify-center rounded-xl bg-selected font-mono text-[13px] font-bold text-fg-link"
                >
                  {f.icon}
                </span>
                <h3 className="text-heading-3">{m.features[f.key].title}</h3>
                <p className="text-body-md text-fg-muted">{m.features[f.key].text}</p>
              </article>
            ))}
          </div>
        </section>

        <section
          aria-labelledby="landing-integration"
          className="grid grid-cols-1 items-center gap-8 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)] lg:gap-12"
        >
          <div className="flex flex-col gap-6">
            <div className="flex flex-col gap-2">
              <h2 id="landing-integration" className={H2}>
                {m.integration.title}
              </h2>
              <p className="text-base leading-6 text-fg-muted">{m.integration.lead}</p>
            </div>
            <ol aria-label={m.integration.stepsLabel} className="flex flex-col gap-3">
              {STEPS.map((key, i) => (
                <li key={key} className="flex items-start gap-3">
                  <span
                    aria-hidden="true"
                    className="flex size-7 shrink-0 items-center justify-center rounded-full bg-selected text-sm font-bold text-fg-link"
                  >
                    {i + 1}
                  </span>
                  <span className="pt-1 text-body-md">{m.integration.steps[key]}</span>
                </li>
              ))}
            </ol>
          </div>
          {/* min-w-0 не даёт длинным строкам распереть колонку: код прокручивается внутри блока. */}
          <figure aria-label={m.integration.snippetLabel} className="min-w-0">
            <pre
              // biome-ignore lint/a11y/noNoninteractiveTabindex: прокручиваемая область должна быть доступна с клавиатуры
              tabIndex={0}
              className="overflow-x-auto rounded-2xl bg-code-bg p-4 font-mono text-[13px] leading-5 text-code-fg shadow-(--shadow-card) outline-none focus-visible:ring-3 focus-visible:ring-ring/50 md:p-6"
            >
              {SNIPPET}
            </pre>
          </figure>
        </section>

        <section
          aria-labelledby="landing-admin"
          className="flex flex-col items-stretch gap-5 rounded-2xl bg-linear-135 from-(--primary-top) to-(--primary-hover) p-6 text-primary-foreground shadow-(--shadow-primary) md:flex-row md:items-center md:justify-between md:gap-8 md:p-8"
        >
          <div className="flex flex-col gap-2">
            <h2 id="landing-admin" className="text-2xl font-extrabold tracking-[-0.02em]">
              {m.admin.title}
            </h2>
            <p className="max-w-[640px] text-base leading-6">{m.admin.text}</p>
          </div>
          <Link
            href="/login"
            className="inline-flex h-11 w-full shrink-0 items-center justify-center rounded-xl bg-white px-5 text-[15px] font-semibold text-[#2d55c7] outline-none hover:bg-white/90 focus-visible:ring-3 focus-visible:ring-white/60 md:w-auto"
          >
            {m.admin.signIn}
          </Link>
        </section>
      </main>

      <footer className="mx-auto flex w-full max-w-[1200px] flex-wrap items-center gap-2 px-4 pb-8 text-caption text-fg-muted md:px-8">
        <span>© 2026 appmetrica.online</span>
        <span aria-hidden="true">·</span>
        <span>
          <a href="https://db-ip.com" className="text-fg-link hover:underline">
            IP Geolocation by DB-IP
          </a>{" "}
          (CC BY 4.0)
        </span>
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
