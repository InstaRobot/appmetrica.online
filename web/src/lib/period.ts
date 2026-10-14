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
