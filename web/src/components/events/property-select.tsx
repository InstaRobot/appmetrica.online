"use client";

import { Suspense } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/common/lazy-select";
import { Skeleton } from "@/components/ui/skeleton";
import type { EventPropertyKey } from "@/lib/api/gen/model";
import { useMessages } from "@/lib/i18n/client";

type PropertySelectProps = {
  /** property_keys отчёта: события с ключом по убыванию. */
  keys: readonly EventPropertyKey[];
  value: string;
  onChange: (key: string) => void;
};

// Выбор свойства для разбивки. Radix Select грузится отдельным чанком (бюджет initial JS), пока он
// не пришёл, на его месте поле того же размера. Ключ из URL, которого нет в отчёте, остаётся в списке:
// иначе выбранное значение пропало бы из селекта, хотя таблица по нему построена.
export function PropertySelect({ keys, value, onChange }: PropertySelectProps) {
  const m = useMessages().events.event.breakdown;
  const options = keys.some((k) => k.key === value)
    ? keys.map((k) => k.key)
    : [value, ...keys.map((k) => k.key)];

  return (
    <Suspense fallback={<Skeleton className="h-8 w-44" />}>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger aria-label={m.property} className="w-44 font-mono">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((key) => (
            <SelectItem key={key} value={key} className="font-mono">
              {key}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Suspense>
  );
}
