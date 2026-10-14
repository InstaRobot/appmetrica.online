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

// TODO(T-037): заглушки форматирования дат и дельты.
export function formatDelta(_value: number | null | undefined): string {
  return "";
}

export function formatDate(_value: string, _timeZone?: string): string {
  return "";
}

export function formatDateShort(_value: string): string {
  return "";
}

export function formatWeekdayDate(_value: string): string {
  return "";
}
