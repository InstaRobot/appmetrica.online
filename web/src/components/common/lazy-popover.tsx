"use client";

import { lazy } from "react";

// Ленивые части shadcn Popover (Radix Popover, react-remove-scroll и aria-hidden, ≈ 10 KB gz).
// Тулбар отчёта (PeriodPicker, FilterBar) рендерит кнопку сразу, а код popover догружается после
// первой загрузки страницы (бюджет initial JS, NFR-006). Popover оборачивается в
// <Suspense fallback={кнопка того же вида}>: до загрузки чанка видна обычная кнопка.
const chunk = () => import("@/components/ui/popover");

export const Popover = lazy(() => chunk().then((m) => ({ default: m.Popover })));
export const PopoverTrigger = lazy(() => chunk().then((m) => ({ default: m.PopoverTrigger })));
export const PopoverContent = lazy(() => chunk().then((m) => ({ default: m.PopoverContent })));
