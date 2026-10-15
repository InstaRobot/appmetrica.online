"use client";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useMessages } from "@/lib/i18n/client";
import { GRANULARITIES, type Granularity } from "@/lib/period";

type GranularitySelectProps = {
  value: Granularity;
  /** allowedGranularities(period). */
  allowed: Granularity[];
  onChange: (value: Granularity) => void;
};

// Детализация графика (14-ux-ui-spec.md §16.7): недоступные для периода варианты выключены.
export function GranularitySelect({ value, allowed, onChange }: GranularitySelectProps) {
  const m = useMessages().reports.granularity;
  return (
    <ToggleGroup
      type="single"
      variant="segmented"
      size="sm"
      spacing={0.5}
      aria-label={m.label}
      value={value}
      onValueChange={(next) => {
        const g = GRANULARITIES.find((x) => x === next);
        if (g && allowed.includes(g)) onChange(g);
      }}
    >
      {GRANULARITIES.map((g) => (
        <ToggleGroupItem key={g} value={g} disabled={!allowed.includes(g)} className="px-3">
          {m.options[g]}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
