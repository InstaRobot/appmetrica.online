"use client";

import { useId } from "react";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

type CompareToggleProps = {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  /** Причина, по которой сравнение недоступно: переключатель выключен и объясняет её подсказкой. */
  disabledReason?: string;
};

// Сравнение с предыдущим периодом той же длины (14-ux-ui-spec.md §16.7): пунктир на графике.
// Недоступное сравнение — aria-disabled, а не disabled: переключатель остаётся в порядке фокуса, чтобы
// подсказку с причиной открывали и мышь, и клавиатура.
export function CompareToggle({ checked, onCheckedChange, disabledReason }: CompareToggleProps) {
  const id = useId();
  const reasonId = useId();
  if (!disabledReason) {
    return (
      <div className="flex items-center gap-2">
        <Switch
          id={id}
          checked={checked}
          onCheckedChange={onCheckedChange}
          aria-label="Сравнить с предыдущим периодом"
        />
        <Label htmlFor={id}>Сравнить</Label>
      </div>
    );
  }
  return (
    <div className="flex items-center gap-2">
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Switch
              id={id}
              checked={false}
              aria-disabled="true"
              aria-describedby={reasonId}
              className="cursor-not-allowed opacity-50"
              aria-label="Сравнить с предыдущим периодом"
            />
          </TooltipTrigger>
          <TooltipContent>{disabledReason}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
      <Label htmlFor={id} className="opacity-50">
        Сравнить
      </Label>
      <span id={reasonId} className="sr-only">
        {disabledReason}
      </span>
    </div>
  );
}
