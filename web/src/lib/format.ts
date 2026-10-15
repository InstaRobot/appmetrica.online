import type { Locale } from "@/lib/i18n/locale";

// Форматирование чисел и дат интерфейса (14-ux-ui-spec.md) на языке пользователя. RU: разделитель групп и
// пробел перед «%» и единицами — неразрывный U+00A0, месяцы «окт»; EN: en-US, «Oct 12, 2026», «38.1%».
// Время везде 24-часовое. Отсутствующее значение — «—». Набор функций на язык — createFormatters(locale),
// на клиенте — useFormat().
const NBSP = " ";
const DASH = "—";

// Часть движков ICU ставит узкий неразрывный пробел U+202F — приводим к U+00A0 для стабильного вывода.
const normalizeSpaces = (s: string) => s.replace(/[  ]/g, NBSP);

const isNumber = (value: number | null | undefined): value is number =>
  typeof value === "number" && Number.isFinite(value);

type DateParts = { year: number; month: number; day: number; weekday: number };

type LocaleTable = {
  intl: string;
  // Сокращения месяцев по 14-ux-ui-spec.md §14.2: Intl даёт «окт.» и «сент.», поэтому свои.
  months: readonly string[];
  weekdays: readonly string[];
  units: { h: string; m: string; s: string };
  // Разделитель числа и единицы длительности: «4 мин 12 с» и «4m 12s».
  unitGap: string;
  // Знак процента у дельты: «+4,1 %» и «+4.1%».
  percentGap: string;
  // День без года: «12 окт», «пн 12 окт» / «Oct 12», «Mon, Oct 12».
  day: (p: { day: number; month: string; weekday?: string }) => string;
  // Дни без года: «6–12 окт», «28 сен – 5 окт» / «Oct 6–12», «Sep 28 – Oct 5».
  span: (a: { day: number; month: string }, b: { day: number; month: string }) => string;
  withYear: (text: string, year: number) => string;
  dateTime: (date: string, time: string) => string;
};

const TABLES: Record<Locale, LocaleTable> = {
  ru: {
    intl: "ru-RU",
    months: ["янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"],
    weekdays: ["вс", "пн", "вт", "ср", "чт", "пт", "сб"],
    units: { h: "ч", m: "мин", s: "с" },
    unitGap: NBSP,
    percentGap: NBSP,
    day: ({ day, month, weekday }) => `${weekday ? `${weekday}${NBSP}` : ""}${day}${NBSP}${month}`,
    span: (a, b) =>
      a.month === b.month
        ? `${a.day}–${b.day}${NBSP}${b.month}`
        : `${a.day}${NBSP}${a.month} – ${b.day}${NBSP}${b.month}`,
    withYear: (text, year) => `${text}${NBSP}${year}`,
    dateTime: (date, time) => `${date} ${time}`,
  },
  en: {
    intl: "en-US",
    months: ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"],
    weekdays: ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"],
    units: { h: "h", m: "m", s: "s" },
    unitGap: "",
    percentGap: "",
    day: ({ day, month, weekday }) => `${weekday ? `${weekday}, ` : ""}${month}${NBSP}${day}`,
    span: (a, b) =>
      a.month === b.month
        ? `${a.month}${NBSP}${a.day}–${b.day}`
        : `${a.month}${NBSP}${a.day} – ${b.month}${NBSP}${b.day}`,
    withYear: (text, year) => `${text}, ${year}`,
    dateTime: (date, time) => `${date}, ${time}`,
  },
};

const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;

const partsFormats = new Map<string, Intl.DateTimeFormat>();

function partsFormat(timeZone: string): Intl.DateTimeFormat {
  let format = partsFormats.get(timeZone);
  if (!format) {
    format = new Intl.DateTimeFormat("en-US", {
      timeZone,
      year: "numeric",
      month: "numeric",
      day: "numeric",
    });
    partsFormats.set(timeZone, format);
  }
  return format;
}

// Дата YYYY-MM-DD — календарный день отчёта как есть; момент времени (ISO с временем) — день в timeZone.
function dateParts(value: string, timeZone?: string): DateParts | null {
  const iso = ISO_DATE.exec(value);
  const instant = iso
    ? new Date(Date.UTC(Number(iso[1]), Number(iso[2]) - 1, Number(iso[3])))
    : new Date(value);
  if (Number.isNaN(instant.getTime())) return null;
  let zone = iso ? "UTC" : (timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone);
  let parts: Intl.DateTimeFormatPart[];
  try {
    parts = partsFormat(zone).formatToParts(instant);
  } catch {
    zone = "UTC";
    parts = partsFormat(zone).formatToParts(instant);
  }
  const get = (type: string) => Number(parts.find((p) => p.type === type)?.value);
  const year = get("year");
  const month = get("month");
  const day = get("day");
  return { year, month, day, weekday: new Date(Date.UTC(year, month - 1, day)).getUTCDay() };
}

function make(locale: Locale) {
  const t = TABLES[locale];
  const month = (m: number) => t.months[m - 1] ?? "";

  const intFormat = new Intl.NumberFormat(t.intl, { maximumFractionDigits: 0 });
  const percentFormat = new Intl.NumberFormat(t.intl, {
    style: "percent",
    maximumFractionDigits: 1,
  });
  const shareFormat = new Intl.NumberFormat(t.intl, { style: "percent", maximumFractionDigits: 0 });
  const rateFormat = new Intl.NumberFormat(t.intl, {
    style: "percent",
    maximumSignificantDigits: 2,
  });
  const deltaFormat = new Intl.NumberFormat(t.intl, { maximumFractionDigits: 1 });
  const timeFormats = new Map<string, Intl.DateTimeFormat>();

  // Время «14:32» или «14:32:05» в timeZone; неизвестная зона — UTC.
  function timeFormat(timeZone: string, seconds = false): Intl.DateTimeFormat {
    const key = `${timeZone}|${seconds}`;
    let format = timeFormats.get(key);
    if (!format) {
      let zone = timeZone;
      try {
        new Intl.DateTimeFormat(t.intl, { timeZone: zone });
      } catch {
        zone = "UTC";
      }
      format = new Intl.DateTimeFormat(t.intl, {
        timeZone: zone,
        hour: "2-digit",
        minute: "2-digit",
        ...(seconds ? { second: "2-digit" } : {}),
        hourCycle: "h23",
      });
      timeFormats.set(key, format);
    }
    return format;
  }

  function formatInstant(ts: string, timeZone: string, seconds: boolean): string {
    const date = new Date(ts);
    return Number.isNaN(date.getTime()) ? DASH : timeFormat(timeZone, seconds).format(date);
  }

  // «12 окт 2026» / «Oct 12, 2026». Дата YYYY-MM-DD или момент времени ISO 8601 (тогда день — в timeZone).
  function formatDate(value: string, timeZone?: string): string {
    const p = dateParts(value, timeZone);
    return p ? t.withYear(t.day({ day: p.day, month: month(p.month) }), p.year) : DASH;
  }

  // «12 окт» / «Oct 12» — заголовки колонок и оси графиков.
  function formatDateShort(value: string, timeZone?: string): string {
    const p = dateParts(value, timeZone);
    return p ? t.day({ day: p.day, month: month(p.month) }) : DASH;
  }

  function formatTime(ts: string, timeZone: string): string {
    return formatInstant(ts, timeZone, false);
  }

  return {
    formatInt(value: number | null | undefined): string {
      return isNumber(value) ? normalizeSpaces(intFormat.format(value)) : DASH;
    },

    // Доля 0..1 → «38,1 %» / «38.1%».
    formatPercent(value: number | null | undefined): string {
      return isNumber(value) ? normalizeSpaces(percentFormat.format(value)) : DASH;
    },

    // Доля 0..1 целыми процентами → «51 %» / «51%» (доли платформ в портфеле).
    formatShare(value: number | null | undefined): string {
      return isNumber(value) ? normalizeSpaces(shareFormat.format(value)) : DASH;
    },

    // Доля отказов rejected / (accepted + rejected) с двумя значащими цифрами: «0,29 %», «0,004 %».
    formatRejectionRate(rejected: number, accepted: number): string {
      const total = accepted + rejected;
      return total > 0 ? normalizeSpaces(rateFormat.format(rejected / total)) : DASH;
    },

    // Длительность в миллисекундах → две старшие единицы: «4 мин 12 с» / «4m 12s», «1 ч 2 мин» / «1h 2m».
    formatDuration(ms: number | null | undefined): string {
      if (!isNumber(ms) || ms < 0) return DASH;
      const total = Math.round(ms / 1000);
      const h = Math.floor(total / 3600);
      const m = Math.floor((total % 3600) / 60);
      const s = total % 60;
      const unit = (n: number, u: string) => `${n}${t.unitGap}${u}`;

      const parts: string[] = [];
      if (h > 0) {
        parts.push(unit(h, t.units.h));
        if (m > 0) parts.push(unit(m, t.units.m));
      } else if (m > 0) {
        parts.push(unit(m, t.units.m));
        if (s > 0) parts.push(unit(s, t.units.s));
      } else {
        parts.push(unit(s, t.units.s));
      }
      return parts.join(locale === "ru" ? NBSP : " ");
    },

    // Дельта в процентах (4.1 = +4,1 %): знак явный, минус — U+2212; после округления до 0,1 ноль без знака.
    formatDelta(value: number | null | undefined): string {
      if (!isNumber(value)) return DASH;
      const rounded = Math.round(value * 10) / 10;
      const abs = normalizeSpaces(deltaFormat.format(Math.abs(rounded)));
      const sign = rounded > 0 ? "+" : rounded < 0 ? "−" : "";
      return `${sign}${abs}${t.percentGap}%`;
    },

    formatDate,
    formatDateShort,

    // «пн 5 окт» / «Mon, Oct 5» — колонка «Δ к …» портфеля.
    formatWeekdayDate(value: string, timeZone?: string): string {
      const p = dateParts(value, timeZone);
      return p
        ? t.day({ day: p.day, month: month(p.month), weekday: t.weekdays[p.weekday] ?? "" })
        : DASH;
    },

    // Период отчёта: «6–12 окт 2026», «28 сен – 5 окт 2026», «28 дек 2025 – 3 янв 2026», «12 окт 2026».
    formatDateRange(from: string, to: string): string {
      const a = dateParts(from);
      const b = dateParts(to);
      if (!a || !b) return DASH;
      if (from === to) return formatDate(from);
      if (a.year !== b.year) return `${formatDate(from)} – ${formatDate(to)}`;
      return t.withYear(
        t.span({ day: a.day, month: month(a.month) }, { day: b.day, month: month(b.month) }),
        b.year,
      );
    },

    // Когорта ISO-недели по её понедельнику: «8–14 сен», «28 сен – 4 окт» (S-06).
    formatWeekRange(start: string): string {
      const a = dateParts(start);
      if (!a || !ISO_DATE.test(start)) return DASH;
      const end = new Date(Date.UTC(a.year, a.month - 1, a.day + 6));
      return t.span(
        { day: a.day, month: month(a.month) },
        { day: end.getUTCDate(), month: month(end.getUTCMonth() + 1) },
      );
    },

    // Минута момента времени в timeZone: «14:32» (оси realtime-графика).
    formatTime,

    // Время с секундами в timeZone: «14:32:05» (лента и «обновлено» на S-07).
    formatClock(ts: string, timeZone: string): string {
      return formatInstant(ts, timeZone, true);
    },

    // Дата и минута в timeZone: «12 окт 2026 14:01» / «Oct 12, 2026, 14:01» (профиль S-09).
    formatDateTime(ts: string, timeZone: string): string {
      const date = formatDate(ts, timeZone);
      return date === DASH ? DASH : t.dateTime(date, formatTime(ts, timeZone));
    },

    // Длинный идентификатор → «первые4…последние4» (14-ux-ui-spec.md S-07); короткий — как есть.
    shortId(value: string): string {
      if (!value) return DASH;
      return value.length <= 9 ? value : `${value.slice(0, 4)}…${value.slice(-4)}`;
    },

    // Подпись корзины ряда (начало корзины с offset TZ приложения): «12 окт, 14:00», «12 окт», «окт 2026».
    formatBucket(
      ts: string,
      granularity: "hour" | "day" | "week" | "month",
      timeZone: string,
    ): string {
      const p = dateParts(ts, timeZone);
      if (!p) return DASH;
      if (granularity === "month") return `${month(p.month)}${NBSP}${p.year}`;
      const day = formatDateShort(ts, timeZone);
      if (granularity !== "hour") return day;
      return `${day}, ${timeFormat(timeZone).format(new Date(ts))}`;
    },
  };
}

export type Formatters = ReturnType<typeof make>;

const cache = new Map<Locale, Formatters>();

export function createFormatters(locale: Locale): Formatters {
  let formatters = cache.get(locale);
  if (!formatters) {
    formatters = make(locale);
    cache.set(locale, formatters);
  }
  return formatters;
}

// Русские функции модуля — на время перевода экранов: каждый экран переходит на useFormat()/createFormatters.
export const {
  formatInt,
  formatPercent,
  formatShare,
  formatRejectionRate,
  formatDuration,
  formatDelta,
  formatDate,
  formatDateShort,
  formatWeekdayDate,
  formatDateRange,
  formatWeekRange,
  formatTime,
  formatClock,
  formatDateTime,
  shortId,
  formatBucket,
} = createFormatters("ru");
