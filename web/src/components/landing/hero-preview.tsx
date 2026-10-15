import { createFormatters } from "@/lib/format";
import type { Locale } from "@/lib/i18n/locale";
import { getMessages } from "@/messages";

// Превью дашборда на лендинге (S-00): статичная карточка «Notes iOS» с тремя KPI и графиком активных
// за 30 дней. Данные иллюстративные и не зависят от времени: страница пререндерится при сборке.
const SERIES = Array.from({ length: 30 }, (_, i) => {
  const trend = 10_400 + i * 70;
  const weekly = [0, 220, 340, 300, 180, -520, -640][i % 7] ?? 0;
  const wave = Math.round(Math.sin(i / 2.3) * 180);
  return trend + weekly + wave;
});

const KPIS = [
  { label: "active", value: 12_480, delta: 4.1 },
  { label: "new", value: 1_204, delta: 2.3 },
  { label: "sessions", value: 31_022, delta: 3.8 },
] as const;

function paths(series: number[], width: number, height: number) {
  const max = Math.max(...series) * 1.1;
  const min = Math.min(...series) * 0.8;
  const step = width / (series.length - 1);
  const points = series.map((v, i) => {
    const x = Math.round(i * step);
    const y = Math.round(height - ((v - min) / (max - min)) * height);
    return `${x},${y}`;
  });
  const line = `M${points.join(" L")}`;
  return { line, area: `${line} L${width},${height} L0,${height} Z` };
}

const { line, area } = paths(SERIES, 1000, 300);

export function HeroPreview({ locale }: { locale: Locale }) {
  const m = getMessages(locale).landing.preview;
  const f = createFormatters(locale);
  return (
    <figure
      aria-label={m.label}
      className="relative flex flex-col gap-4 rounded-2xl border border-border bg-card p-6 shadow-(--shadow-card)"
    >
      <div className="flex items-center gap-2.5">
        <span className="inline-flex h-[22px] min-w-[34px] items-center justify-center rounded-md bg-muted px-2 text-[11px] font-bold text-fg-muted">
          iOS
        </span>
        <span className="font-bold">Notes iOS</span>
        <span className="ml-auto text-caption text-fg-subtle">{m.period}</span>
      </div>
      <dl className="grid grid-cols-3 gap-3">
        {KPIS.map((k) => (
          <div key={k.label} className="flex flex-col gap-1 rounded-xl bg-subtle p-3.5">
            <dt className="text-caption font-semibold text-fg-subtle">{m[k.label]}</dt>
            <dd className="text-[22px] leading-7 font-extrabold tracking-tight tabular-nums">
              {f.formatInt(k.value)}
            </dd>
            <dd className="text-caption font-semibold text-positive">▲ {f.formatDelta(k.delta)}</dd>
          </div>
        ))}
      </dl>
      <svg
        aria-hidden="true"
        viewBox="0 0 1000 300"
        preserveAspectRatio="none"
        className="h-40 w-full text-chart-1"
      >
        <defs>
          <linearGradient id="landing-hero-area" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="currentColor" stopOpacity="0.38" />
            <stop offset="1" stopColor="currentColor" stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={area} fill="url(#landing-hero-area)" />
        <path
          d={line}
          fill="none"
          stroke="currentColor"
          strokeWidth="3"
          vectorEffect="non-scaling-stroke"
        />
      </svg>
      <figcaption className="flex justify-between text-caption text-fg-subtle">
        <span>{m.from}</span>
        <span>{m.chart}</span>
        <span>{m.to}</span>
      </figcaption>
      <span className="absolute -top-3 right-6 inline-flex h-6 items-center gap-1.5 rounded-full bg-linear-to-b from-(--primary-top) to-primary px-2.5 text-[11px] font-bold text-primary-foreground shadow-(--shadow-primary)">
        <span
          aria-hidden="true"
          className="size-1.5 rounded-full bg-white motion-safe:animate-pulse"
        />
        live
      </span>
    </figure>
  );
}
