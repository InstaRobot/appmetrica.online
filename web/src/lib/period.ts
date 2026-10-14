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

// TODO(T-038): заглушки периодов отчётов.
export const PERIOD_PRESETS = ["today", "yesterday", "7d", "30d", "90d", "custom"] as const;
export type PeriodPreset = (typeof PERIOD_PRESETS)[number];
export type PeriodValue = { preset: PeriodPreset; from: string; to: string };
export const GRANULARITIES = ["hour", "day", "week", "month"] as const;
export type Granularity = (typeof GRANULARITIES)[number];

export function presetPeriod(
  _preset: Exclude<PeriodPreset, "custom">,
  _today: string,
): PeriodValue {
  throw new Error("not implemented");
}

export function rangeDays(_from: string, _to: string): number {
  throw new Error("not implemented");
}

export function allowedGranularities(_period: PeriodValue): Granularity[] {
  throw new Error("not implemented");
}

export function coerceGranularity(_value: Granularity, _allowed: Granularity[]): Granularity {
  throw new Error("not implemented");
}
