"use client";

import { useId } from "react";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

type CompareToggleProps = { checked: boolean; onCheckedChange: (checked: boolean) => void };

// Сравнение с предыдущим периодом той же длины (14-ux-ui-spec.md §16.7): пунктир на графике.
export function CompareToggle({ checked, onCheckedChange }: CompareToggleProps) {
  const id = useId();
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
