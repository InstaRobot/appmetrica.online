"use client";

import { Plus, X } from "lucide-react";
import { Suspense, useState } from "react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSkeleton,
} from "@/components/common/lazy-command";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { formatInt } from "@/lib/format";
import { FILTER_DIMENSIONS, type FilterDimension, type Filters } from "@/lib/url-state";

type FilterOption = { value: string; count: number };

type FilterBarProps = {
  value: Filters;
  onChange: (value: Filters) => void;
  /** Значения для выбора — из getAudienceBreakdown по текущему периоду (limit 50). */
  options: Partial<Record<FilterDimension, FilterOption[]>>;
  /** Открытие списка: значения загружаются только тогда, когда они нужны. */
  onOpenChange?: (open: boolean) => void;
  loading?: boolean;
};

export const FILTER_LABELS: Record<FilterDimension, string> = {
  platform: "Платформа",
  app_version: "Версия приложения",
  os_name: "ОС",
  country: "Страна",
};

// toggleFilter добавляет значение в фильтр или убирает его; пустой фильтр удаляется.
export function toggleFilter(filters: Filters, dim: FilterDimension, value: string): Filters {
  const current: string[] = filters[dim] ?? [];
  const values = current.includes(value) ? current.filter((v) => v !== value) : [...current, value];
  const { [dim]: _, ...rest } = filters;
  return values.length > 0 ? ({ ...rest, [dim]: values } as Filters) : rest;
}

// Фильтры отчёта (14-ux-ui-spec.md §16.8): OR внутри измерения, AND между измерениями.
export function FilterBar({
  value,
  onChange,
  options,
  onOpenChange,
  loading = false,
}: FilterBarProps) {
  const [open, setOpen] = useState(false);
  const active = FILTER_DIMENSIONS.filter((dim) => (value[dim]?.length ?? 0) > 0);

  const changeOpen = (next: boolean) => {
    setOpen(next);
    onOpenChange?.(next);
  };

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Popover open={open} onOpenChange={changeOpen}>
        <PopoverTrigger asChild>
          <Button variant="outline" size="sm">
            <Plus aria-hidden="true" />
            Фильтр
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-72 p-0">
          <Suspense fallback={<CommandSkeleton />}>
            <Command>
              <CommandInput placeholder="Найти значение…" aria-label="Найти значение" />
              <CommandList>
                {loading ? (
                  <p className="px-3 py-4 text-body-sm text-fg-muted">Загружаем значения…</p>
                ) : (
                  <CommandEmpty>Ничего не найдено</CommandEmpty>
                )}
                {FILTER_DIMENSIONS.map((dim) => {
                  // Пустое значение («Не определено») фильтром не задаётся: у страны его нет в контракте.
                  const items = (options[dim] ?? []).filter((o) => o.value !== "");
                  if (items.length === 0) return null;
                  return (
                    <CommandGroup key={dim} heading={FILTER_LABELS[dim]}>
                      {items.map((o) => {
                        const selected: string[] = value[dim] ?? [];
                        const checked = selected.includes(o.value);
                        return (
                          <CommandItem
                            key={o.value}
                            value={`${dim}:${o.value}`}
                            keywords={[o.value, FILTER_LABELS[dim]]}
                            data-checked={checked ? "true" : "false"}
                            onSelect={() => {
                              onChange(toggleFilter(value, dim, o.value));
                              changeOpen(false);
                            }}
                          >
                            {o.value}
                            <span className="ml-auto text-fg-muted tabular-nums">
                              {formatInt(o.count)}
                            </span>
                          </CommandItem>
                        );
                      })}
                    </CommandGroup>
                  );
                })}
              </CommandList>
            </Command>
          </Suspense>
        </PopoverContent>
      </Popover>
      {active.map((dim) => (
        <Badge key={dim} variant="secondary" className="h-7 gap-1 pr-1 text-label-sm">
          <span>
            {FILTER_LABELS[dim]}: {value[dim]?.join(", ")}
          </span>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={`Убрать фильтр «${FILTER_LABELS[dim]}»`}
            onClick={() => {
              const { [dim]: _, ...rest } = value;
              onChange(rest);
            }}
          >
            <X aria-hidden="true" />
          </Button>
        </Badge>
      ))}
      {active.length > 0 ? (
        <Button variant="link" size="sm" onClick={() => onChange({})}>
          Сбросить фильтры
        </Button>
      ) : null}
    </div>
  );
}
