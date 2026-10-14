"use client";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { GRANULARITIES, type Granularity } from "@/lib/period";

type GranularitySelectProps = {
  value: Granularity;
  /** allowedGranularities(period). */
  allowed: Granularity[];
  onChange: (value: Granularity) => void;
};

export const GRANULARITY_LABELS: Record<Granularity, string> = {
  hour: "час",
  day: "день",
  week: "неделя",
  month: "месяц",
};

// Детализация графика (14-ux-ui-spec.md §16.7): недоступные для периода варианты выключены.
export function GranularitySelect({ value, allowed, onChange }: GranularitySelectProps) {
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      spacing={0}
      aria-label="Детализация"
      value={value}
      onValueChange={(next) => {
        const g = GRANULARITIES.find((x) => x === next);
        if (g && allowed.includes(g)) onChange(g);
      }}
    >
      {GRANULARITIES.map((g) => (
        <ToggleGroupItem key={g} value={g} disabled={!allowed.includes(g)} className="px-3">
          {GRANULARITY_LABELS[g]}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
