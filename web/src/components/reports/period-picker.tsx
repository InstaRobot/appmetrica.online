"use client";

import { CalendarDays } from "lucide-react";
import { Suspense, useState } from "react";
import type { DateRange } from "react-day-picker";
import { LazyCalendar } from "@/components/common/lazy-calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/common/lazy-popover";
import { Button } from "@/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import {
  addDays,
  MAX_RANGE_DAYS,
  type PeriodPreset,
  type PeriodValue,
  presetPeriod,
  todayIn,
} from "@/lib/period";

type PeriodPickerProps = {
  value: PeriodValue;
  onChange: (value: PeriodValue) => void;
  /** TZ приложения: «сегодня» и пресеты считаются в ней. */
  timezone: string;
  maxRangeDays?: number;
  /** Текст range_too_large — inline под выбором периода. */
  error?: string | undefined;
  /** Доступные пресеты; по умолчанию — все. */
  presets?: readonly Preset[];
};

type Preset = Exclude<PeriodPreset, "custom">;

const PRESETS: readonly Preset[] = ["today", "yesterday", "7d", "30d", "90d"];

const isPreset = (value: string): value is Preset => PRESETS.some((p) => p === value);

// Местная полночь для react-day-picker ↔ YYYY-MM-DD: календарь работает с датами без зоны.
function toPickerDate(date: string): Date {
  const [y, m, d] = date.split("-").map(Number);
  return new Date(y ?? 1970, (m ?? 1) - 1, d ?? 1);
}

function fromPickerDate(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

// Выбор периода S-05 (14-ux-ui-spec.md §16.7): пресеты и произвольный диапазон в календаре.
export function PeriodPicker({
  value,
  onChange,
  timezone,
  maxRangeDays = MAX_RANGE_DAYS,
  error,
  presets,
}: PeriodPickerProps) {
  const m = useMessages().reports.period;
  const f = useFormat();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<DateRange | undefined>();
  const today = todayIn(timezone);
  const custom = value.preset === "custom";
  const range = f.formatDateRange(value.from, value.to);

  const trigger = (
    <Button
      variant={custom ? "secondary" : "outline"}
      size="sm"
      aria-label={custom ? fmt(m.picked, { range }) : m.pick}
    >
      <CalendarDays aria-hidden="true" />
      {custom ? range : null}
    </Button>
  );

  const apply = () => {
    if (!draft?.from) return;
    const from = fromPickerDate(draft.from);
    onChange({ preset: "custom", from, to: draft.to ? fromPickerDate(draft.to) : from });
    setOpen(false);
  };

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-2">
        <ToggleGroup
          type="single"
          variant="segmented"
          size="sm"
          spacing={0.5}
          aria-label={m.label}
          value={custom ? "" : value.preset}
          onValueChange={(next) => {
            // Повторный клик по выбранному пресету даёт "" — период всегда выбран.
            if (isPreset(next) && next !== value.preset) onChange(presetPeriod(next, today));
          }}
        >
          {PRESETS.filter((p) => !presets || presets.includes(p)).map((p) => (
            <ToggleGroupItem key={p} value={p} className="px-3">
              {m.presets[p]}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Suspense fallback={trigger}>
          <Popover
            open={open}
            onOpenChange={(next) => {
              setOpen(next);
              if (next) setDraft(undefined);
            }}
          >
            <PopoverTrigger asChild>{trigger}</PopoverTrigger>
            <PopoverContent align="start" className="w-auto p-0">
              <LazyCalendar
                mode="range"
                numberOfMonths={2}
                showOutsideDays={false}
                selected={draft}
                onSelect={setDraft}
                defaultMonth={toPickerDate(addDays(today, -31))}
                disabled={{
                  after: toPickerDate(today),
                  before: toPickerDate(addDays(today, 1 - 25 * 31)),
                }}
                max={maxRangeDays}
              />
              <div className="flex items-center justify-end gap-2 border-t border-border p-3">
                <Button size="sm" disabled={!draft?.from} onClick={apply}>
                  {m.apply}
                </Button>
              </div>
            </PopoverContent>
          </Popover>
        </Suspense>
      </div>
      {error ? (
        <p role="alert" className="text-body-sm text-negative">
          {error}
        </p>
      ) : null}
    </div>
  );
}
