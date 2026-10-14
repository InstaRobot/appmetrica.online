"use client";

import { lazy } from "react";
import { Skeleton } from "@/components/ui/skeleton";

// Ленивые части shadcn Command (cmdk и Radix Dialog из components/ui/command.tsx, ≈ 8 KB gz).
// Поиск по списку нужен только в открытом popover (AppSwitcher, FilterBar, TimezoneSelect), поэтому
// cmdk грузится одним чанком при первом открытии (бюджет initial JS, NFR-006, T-044). Содержимое
// popover оборачивается в <Suspense fallback={<CommandSkeleton />}>.
const chunk = () => import("@/components/ui/command");

export const Command = lazy(() => chunk().then((m) => ({ default: m.Command })));
export const CommandInput = lazy(() => chunk().then((m) => ({ default: m.CommandInput })));
export const CommandList = lazy(() => chunk().then((m) => ({ default: m.CommandList })));
export const CommandEmpty = lazy(() => chunk().then((m) => ({ default: m.CommandEmpty })));
export const CommandGroup = lazy(() => chunk().then((m) => ({ default: m.CommandGroup })));
export const CommandItem = lazy(() => chunk().then((m) => ({ default: m.CommandItem })));
export const CommandSeparator = lazy(() => chunk().then((m) => ({ default: m.CommandSeparator })));

export function CommandSkeleton() {
  return <Skeleton role="status" aria-label="Загрузка…" className="m-2 h-24" />;
}
