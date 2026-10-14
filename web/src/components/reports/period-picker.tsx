"use client";

import { CalendarDays } from "lucide-react";
import { useState } from "react";
import type { DateRange } from "react-day-picker";
import { LazyCalendar } from "@/components/common/lazy-calendar";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { formatDateRange } from "@/lib/format";
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

const PRESETS: { value: Preset; label: string }[] = [
  { value: "today", label: "Сегодня" },
  { value: "yesterday", label: "Вчера" },
  { value: "7d", label: "7 дней" },
  { value: "30d", label: "30 дней" },
  { value: "90d", label: "90 дней" },
];

const isPreset = (value: string): value is Preset => PRESETS.some((p) => p.value === value);

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
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<DateRange | undefined>();
  const today = todayIn(timezone);
  const custom = value.preset === "custom";
  const range = formatDateRange(value.from, value.to);

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
          variant="outline"
          size="sm"
          spacing={0}
          aria-label="Период"
          value={custom ? "" : value.preset}
          onValueChange={(next) => {
            // Повторный клик по выбранному пресету даёт "" — период всегда выбран.
            if (isPreset(next) && next !== value.preset) onChange(presetPeriod(next, today));
          }}
        >
          {PRESETS.filter((p) => !presets || presets.includes(p.value)).map((p) => (
            <ToggleGroupItem key={p.value} value={p.value} className="px-3">
              {p.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Popover
          open={open}
          onOpenChange={(next) => {
            setOpen(next);
            if (next) setDraft(undefined);
          }}
        >
          <PopoverTrigger asChild>
            <Button
              variant={custom ? "secondary" : "outline"}
              size="sm"
              aria-label={custom ? `Выбрать даты: ${range}` : "Выбрать даты"}
            >
              <CalendarDays aria-hidden="true" />
              {custom ? range : null}
            </Button>
          </PopoverTrigger>
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
                Применить
              </Button>
            </div>
          </PopoverContent>
        </Popover>
      </div>
      {error ? (
        <p role="alert" className="text-body-sm text-negative">
          {error}
        </p>
      ) : null}
    </div>
  );
}
