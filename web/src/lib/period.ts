// Календарные даты отчётов YYYY-MM-DD (14-ux-ui-spec.md §9): арифметика в UTC, «сегодня» — в TZ владельца
// или приложения. Неизвестная TZ (например, ещё не загруженные настройки) считается UTC.
const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;

function toIso(date: Date): string {
  return date.toISOString().slice(0, 10);
}

function parse(date: string): Date {
  const m = ISO_DATE.exec(date);
  return m
    ? new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])))
    : new Date(Number.NaN);
}

// isIsoDate — строка YYYY-MM-DD с существующей календарной датой (2026-02-30 — нет).
export function isIsoDate(value: string): boolean {
  const date = parse(value);
  return !Number.isNaN(date.getTime()) && toIso(date) === value;
}

export function addDays(date: string, days: number): string {
  const d = parse(date);
  d.setUTCDate(d.getUTCDate() + days);
  return toIso(d);
}

export function todayIn(timeZone: string, now: Date = new Date()): string {
  let zone = timeZone;
  try {
    new Intl.DateTimeFormat("en-CA", { timeZone: zone });
  } catch {
    zone = "UTC";
  }
  // en-CA форматирует дату как YYYY-MM-DD.
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(now);
}

export function yesterdayIn(timeZone: string, now: Date = new Date()): string {
  return addDays(todayIn(timeZone, now), -1);
}

// Периоды отчётов S-05 (14-ux-ui-spec.md S-05, §16.7): пресеты заканчиваются сегодняшним днём в TZ приложения
// включительно, как «Неделя» в AppMetrica; custom — произвольные from…to.
export const PERIOD_PRESETS = ["today", "yesterday", "7d", "30d", "90d", "custom"] as const;
export type PeriodPreset = (typeof PERIOD_PRESETS)[number];
export type PeriodValue = { preset: PeriodPreset; from: string; to: string };

export const GRANULARITIES = ["hour", "day", "week", "month"] as const;
export type Granularity = (typeof GRANULARITIES)[number];

// Пределы API (06-api-contracts.md §2.4.4, OQ-042): период до 366 дней, почасовой — до 7 дней.
export const MAX_RANGE_DAYS = 366;
export const MAX_HOUR_RANGE_DAYS = 7;
const MIN_MONTH_RANGE_DAYS = 60;

const PRESET_DAYS = { "7d": 7, "30d": 30, "90d": 90 } as const;

export function presetPeriod(preset: Exclude<PeriodPreset, "custom">, today: string): PeriodValue {
  if (preset === "today") return { preset, from: today, to: today };
  if (preset === "yesterday") {
    const day = addDays(today, -1);
    return { preset, from: day, to: day };
  }
  return { preset, from: addDays(today, 1 - PRESET_DAYS[preset]), to: today };
}

// rangeDays — длина from…to в днях включительно; 0, если from позже to.
export function rangeDays(from: string, to: string): number {
  const days = Math.round((parse(to).getTime() - parse(from).getTime()) / 86_400_000) + 1;
  return Math.max(days, 0);
}

// Детализация, доступная для периода (14-ux-ui-spec.md S-05).
export function allowedGranularities(period: PeriodValue): Granularity[] {
  switch (period.preset) {
    case "today":
    case "yesterday":
      return ["hour"];
    case "7d":
      return ["hour", "day"];
    case "30d":
      return ["day", "week"];
    case "90d":
      return ["day", "week", "month"];
    case "custom": {
      const days = rangeDays(period.from, period.to);
      return GRANULARITIES.filter(
        (g) =>
          (g !== "hour" || days <= MAX_HOUR_RANGE_DAYS) &&
          (g !== "month" || days >= MIN_MONTH_RANGE_DAYS),
      );
    }
  }
}

// Ближайшая допустимая детализация; при равном расстоянии — более мелкая.
export function coerceGranularity(value: Granularity, allowed: Granularity[]): Granularity {
  if (allowed.includes(value)) return value;
  const rank = GRANULARITIES.indexOf(value);
  let best = allowed[0] ?? "day";
  for (const g of allowed) {
    const d = Math.abs(GRANULARITIES.indexOf(g) - rank);
    if (d < Math.abs(GRANULARITIES.indexOf(best) - rank)) best = g;
  }
  return best;
}
