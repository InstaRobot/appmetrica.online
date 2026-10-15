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
import { Popover, PopoverContent, PopoverTrigger } from "@/components/common/lazy-popover";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { FILTER_DIMENSIONS, type FilterDimension, type Filters } from "@/lib/url-state";

type FilterOption = { value: string; count: number };

type FilterBarProps = {
  /** Измерения в списке и чипах; на страницах приложения платформу выбирает PlatformSwitcher. */
  dims?: readonly FilterDimension[];
  value: Filters;
  onChange: (value: Filters) => void;
  /** Значения для выбора — из getAudienceBreakdown по текущему периоду (limit 50). */
  options: Partial<Record<FilterDimension, FilterOption[]>>;
  /** Открытие списка: значения загружаются только тогда, когда они нужны. */
  onOpenChange?: (open: boolean) => void;
  loading?: boolean;
};

// toggleFilter добавляет значение в фильтр или убирает его; пустой фильтр удаляется.
export function toggleFilter(filters: Filters, dim: FilterDimension, value: string): Filters {
  const current: string[] = filters[dim] ?? [];
  const values = current.includes(value) ? current.filter((v) => v !== value) : [...current, value];
  const { [dim]: _, ...rest } = filters;
  return values.length > 0 ? ({ ...rest, [dim]: values } as Filters) : rest;
}

// Фильтры отчёта (14-ux-ui-spec.md §16.8): OR внутри измерения, AND между измерениями. Сброс снимает только
// измерения dims: фильтры остальных (платформа) остаются.
export function FilterBar({
  dims = FILTER_DIMENSIONS,
  value,
  onChange,
  options,
  onOpenChange,
  loading = false,
}: FilterBarProps) {
  const messages = useMessages().reports;
  const m = messages.filters;
  const labels = messages.dimensions;
  const f = useFormat();
  const [open, setOpen] = useState(false);
  const active = dims.filter((dim) => (value[dim]?.length ?? 0) > 0);
  const kept = Object.fromEntries(
    Object.entries(value).filter(([dim]) => !(dims as readonly string[]).includes(dim)),
  ) as Filters;

  const changeOpen = (next: boolean) => {
    setOpen(next);
    onOpenChange?.(next);
  };

  const trigger = (
    <Button variant="outline" size="sm">
      <Plus aria-hidden="true" />
      {m.add}
    </Button>
  );

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Suspense fallback={trigger}>
        <Popover open={open} onOpenChange={changeOpen}>
          <PopoverTrigger asChild>{trigger}</PopoverTrigger>
          <PopoverContent align="start" className="w-72 p-0">
            <Suspense fallback={<CommandSkeleton />}>
              <Command>
                <CommandInput placeholder={m.searchPlaceholder} aria-label={m.search} />
                <CommandList>
                  {loading ? (
                    <p className="px-3 py-4 text-body-sm text-fg-muted">{m.loading}</p>
                  ) : (
                    <CommandEmpty>{m.empty}</CommandEmpty>
                  )}
                  {dims.map((dim) => {
                    // Пустое значение («Не определено») фильтром не задаётся: у страны его нет в контракте.
                    const items = (options[dim] ?? []).filter((o) => o.value !== "");
                    if (items.length === 0) return null;
                    return (
                      <CommandGroup key={dim} heading={labels[dim]}>
                        {items.map((o) => {
                          const selected: string[] = value[dim] ?? [];
                          const checked = selected.includes(o.value);
                          return (
                            <CommandItem
                              key={o.value}
                              value={`${dim}:${o.value}`}
                              keywords={[o.value, labels[dim]]}
                              data-checked={checked ? "true" : "false"}
                              onSelect={() => {
                                onChange(toggleFilter(value, dim, o.value));
                                changeOpen(false);
                              }}
                            >
                              {o.value}
                              <span className="ml-auto text-fg-muted tabular-nums">
                                {f.formatInt(o.count)}
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
      </Suspense>
      {active.map((dim) => (
        <Badge key={dim} variant="secondary" className="h-7 gap-1 pr-1 text-label-sm">
          <span>
            {labels[dim]}: {value[dim]?.join(", ")}
          </span>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={fmt(m.remove, { dimension: labels[dim] })}
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
        <Button variant="link" size="sm" onClick={() => onChange(kept)}>
          {m.reset}
        </Button>
      ) : null}
    </div>
  );
}
