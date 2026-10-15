import type { CSSProperties } from "react";

// Общие классы адаптивной вёрстки отчётов (mobile-first): базовые классы — телефон (< md), с md — прежние
// компактные размеры десктопа.

// Цели нажатия на телефоне не меньше 44 × 44 px (WCAG 2.2, 2.5.8 с запасом до 2.5.5).
export const TOUCH_TOGGLE = "h-11 min-w-11 md:h-7 md:min-w-7";
export const TOUCH_BUTTON = "h-11 md:h-8";

// Переключатель на всю ширину телефона: пункты делят ширину поровну, с md — по содержимому.
export const STRETCH_GROUP = "w-full md:w-fit";
export const STRETCH_ITEM = "flex-1 md:flex-none";

// Первая колонка плотной таблицы закреплена: при горизонтальной прокрутке цифры уезжают под неё
// (закреплённая ячейка позиционирована и рисуется поверх обычных, z-index не нужен).
export const STICKY_FIRST_COL = "sticky left-0 bg-card";
// Подсветка строки под курсором (bg-muted/50 у TableRow) на непрозрачном фоне закреплённой ячейки:
// строке нужен класс group/row. transition-colors — как у строки, чтобы ячейка не опережала её.
export const STICKY_ROW_HOVER =
  "transition-colors group-hover/row:bg-[color-mix(in_srgb,var(--color-muted)_50%,var(--color-card))]";
// Длинное значение в закреплённой колонке на телефоне обрезается, чтобы цифрам оставалось место.
export const STICKY_VALUE = "block max-w-36 truncate md:max-w-none";

// Высота графика на телефоне — не больше 220 px, с md — заданная. Высота задаётся переменной, а не
// стилем height: брейкпоинт выбирает CSS, без измерения ширины на JS.
export const CHART_HEIGHT = "h-[min(var(--chart-h),220px)] md:h-(--chart-h)";

export function chartHeight(height: number): CSSProperties {
  return { "--chart-h": `${height}px` } as CSSProperties;
}
