// Форматирование чисел для интерфейса в ru-RU (14-ux-ui-spec.md): разделитель групп и пробел перед
// «%» и единицами — неразрывный U+00A0. Отсутствующее значение отображается «—».
const NBSP = " ";
const DASH = "—";

const intFormat = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 0 });
const percentFormat = new Intl.NumberFormat("ru-RU", {
  style: "percent",
  maximumFractionDigits: 1,
});

// Часть движков ICU ставит узкий неразрывный пробел U+202F — приводим к U+00A0 для стабильного вывода.
const normalizeSpaces = (s: string) => s.replace(/[  ]/g, NBSP);

const isNumber = (value: number | null | undefined): value is number =>
  typeof value === "number" && Number.isFinite(value);

export function formatInt(value: number | null | undefined): string {
  return isNumber(value) ? normalizeSpaces(intFormat.format(value)) : DASH;
}

// Доля 0..1 → «38,1 %».
export function formatPercent(value: number | null | undefined): string {
  return isNumber(value) ? normalizeSpaces(percentFormat.format(value)) : DASH;
}

// Длительность в миллисекундах → две старшие единицы: «45 с», «4 мин 12 с», «1 ч 2 мин».
export function formatDuration(ms: number | null | undefined): string {
  if (!isNumber(ms) || ms < 0) return DASH;
  const total = Math.round(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;

  const parts: string[] = [];
  if (h > 0) {
    parts.push(`${h}${NBSP}ч`);
    if (m > 0) parts.push(`${m}${NBSP}мин`);
  } else if (m > 0) {
    parts.push(`${m}${NBSP}мин`);
    if (s > 0) parts.push(`${s}${NBSP}с`);
  } else {
    parts.push(`${s}${NBSP}с`);
  }
  return parts.join(NBSP);
}

const deltaFormat = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 1 });

// Дельта в процентах (4.1 = +4,1 %): знак явный, минус — U+2212; после округления до 0,1 ноль без знака.
export function formatDelta(value: number | null | undefined): string {
  if (!isNumber(value)) return DASH;
  const rounded = Math.round(value * 10) / 10;
  const abs = normalizeSpaces(deltaFormat.format(Math.abs(rounded)));
  const sign = rounded > 0 ? "+" : rounded < 0 ? "−" : "";
  return `${sign}${abs}${NBSP}%`;
}

// Сокращения месяцев по 14-ux-ui-spec.md §14.2 («12 окт 2026», «29 сен»): Intl даёт «окт.» и «сент.».
const MONTHS = ["янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];
const WEEKDAYS = ["вс", "пн", "вт", "ср", "чт", "пт", "сб"];

const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;

type DateParts = { year: number; month: number; day: number; weekday: number };

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

// «12 окт 2026». Принимает дату YYYY-MM-DD или момент времени ISO 8601 (тогда день — в timeZone).
export function formatDate(value: string, timeZone?: string): string {
  const p = dateParts(value, timeZone);
  return p ? `${p.day}${NBSP}${MONTHS[p.month - 1]}${NBSP}${p.year}` : DASH;
}

// «12 окт» — заголовки колонок и оси графиков.
export function formatDateShort(value: string, timeZone?: string): string {
  const p = dateParts(value, timeZone);
  return p ? `${p.day}${NBSP}${MONTHS[p.month - 1]}` : DASH;
}

// «пн 5 окт» — колонка «Δ к …» портфеля.
export function formatWeekdayDate(value: string, timeZone?: string): string {
  const p = dateParts(value, timeZone);
  return p ? `${WEEKDAYS[p.weekday]}${NBSP}${p.day}${NBSP}${MONTHS[p.month - 1]}` : DASH;
}
