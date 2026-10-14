// Время демо-отчётов в TZ приложения без внешних библиотек: смещение зоны берётся из Intl, календарная
// арифметика YYYY-MM-DD — из @/lib/period.
import { addDays } from "@/lib/period";

export const MINUTE_MS = 60_000;
export const HOUR_MS = 3_600_000;
export const DAY_MS = 86_400_000;

const formatters = new Map<string, Intl.DateTimeFormat>();

function formatter(timeZone: string): Intl.DateTimeFormat {
  let f = formatters.get(timeZone);
  if (!f) {
    f = new Intl.DateTimeFormat("en-US", {
      timeZone,
      hourCycle: "h23",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
    formatters.set(timeZone, f);
  }
  return f;
}

export type LocalTime = { date: string; hour: number; minute: number; second: number };

function parts(instant: number, timeZone: string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const p of formatter(timeZone).formatToParts(new Date(instant))) {
    if (p.type !== "literal") out[p.type] = Number(p.value);
  }
  return out;
}

const pad = (n: number, len = 2) => String(n).padStart(len, "0");

export function localTime(instant: number, timeZone: string): LocalTime {
  const p = parts(instant, timeZone);
  return {
    date: `${pad(p.year ?? 1970, 4)}-${pad(p.month ?? 1)}-${pad(p.day ?? 1)}`,
    hour: p.hour ?? 0,
    minute: p.minute ?? 0,
    second: p.second ?? 0,
  };
}

// offsetMs — смещение зоны от UTC в момент instant (Москва: +3 ч).
export function offsetMs(instant: number, timeZone: string): number {
  const p = parts(instant, timeZone);
  const asUtc = Date.UTC(
    p.year ?? 1970,
    (p.month ?? 1) - 1,
    p.day ?? 1,
    p.hour ?? 0,
    p.minute ?? 0,
    p.second ?? 0,
  );
  return asUtc - Math.floor(instant / 1000) * 1000;
}

function utcMidnight(day: string): number {
  const [y = 1970, m = 1, d = 1] = day.split("-").map(Number);
  return Date.UTC(y, m - 1, d);
}

// zonedMidnight — момент начала календарного дня day в зоне (учитывает переход на летнее время).
export function zonedMidnight(day: string, timeZone: string): number {
  const t0 = utcMidnight(day);
  const guess = t0 - offsetMs(t0, timeZone);
  return t0 - offsetMs(guess, timeZone);
}

// formatZoned — RFC 3339 с offset зоны, как time.Time в TZ приложения у Go API: 2026-10-13T12:46:00+03:00.
export function formatZoned(instant: number, timeZone: string): string {
  const off = offsetMs(instant, timeZone);
  const local = new Date(Math.floor(instant / 1000) * 1000 + off).toISOString().slice(0, 19);
  if (off === 0 && (timeZone === "UTC" || timeZone === "Etc/UTC")) return `${local}Z`;
  const abs = Math.abs(off) / MINUTE_MS;
  return `${local}${off < 0 ? "-" : "+"}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`;
}

// isoUtc — момент в UTC с миллисекундами (2026-10-13T09:15:01.990Z), как сырые времена событий в API.
export function isoUtc(instant: number): string {
  return new Date(instant).toISOString();
}

// weekdayIndex — 0 для понедельника … 6 для воскресенья.
export function weekdayIndex(day: string): number {
  return (new Date(utcMidnight(day)).getUTCDay() + 6) % 7;
}

export function mondayOf(day: string): string {
  return addDays(day, -weekdayIndex(day));
}

export function firstOfMonth(day: string): string {
  return `${day.slice(0, 8)}01`;
}

export function addMonths(day: string, months: number): string {
  const [y = 1970, m = 1, d = 1] = day.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1 + months, d)).toISOString().slice(0, 10);
}

// dayDiff — число дней от a до b (b − a), может быть отрицательным.
export function dayDiff(a: string, b: string): number {
  return Math.round((utcMidnight(b) - utcMidnight(a)) / DAY_MS);
}

export function eachDay(from: string, to: string): string[] {
  const out: string[] = [];
  for (let d = from; d <= to; d = addDays(d, 1)) out.push(d);
  return out;
}

export function minDay(a: string, b: string): string {
  return a < b ? a : b;
}

export function maxDay(a: string, b: string): string {
  return a > b ? a : b;
}
