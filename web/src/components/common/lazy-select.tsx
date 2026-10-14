"use client";

import { lazy } from "react";

// Ленивые части shadcn Select (Radix Select с Floating UI, ≈ 6 KB gz). Форма в диалоге (создание
// write-ключа) рендерит Select только после открытия, поэтому код догружается вне initial JS страницы
// (бюджет NFR-006). Select оборачивается в <Suspense fallback={поле того же вида}>.
const chunk = () => import("@/components/ui/select");

export const Select = lazy(() => chunk().then((m) => ({ default: m.Select })));
export const SelectTrigger = lazy(() => chunk().then((m) => ({ default: m.SelectTrigger })));
export const SelectValue = lazy(() => chunk().then((m) => ({ default: m.SelectValue })));
export const SelectContent = lazy(() => chunk().then((m) => ({ default: m.SelectContent })));
export const SelectItem = lazy(() => chunk().then((m) => ({ default: m.SelectItem })));
