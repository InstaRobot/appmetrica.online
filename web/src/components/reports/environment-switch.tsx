"use client";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Environment } from "@/lib/api/gen/model";

type EnvironmentSwitchProps = {
  value: Environment;
  onChange: (value: Environment) => void;
};

const OPTIONS = [Environment.production, Environment.development];

// Переключатель окружения отчётов (14-ux-ui-spec.md §16.7): production / development.
export function EnvironmentSwitch({ value, onChange }: EnvironmentSwitchProps) {
  return (
    <ToggleGroup
      type="single"
      variant="segmented"
      size="sm"
      spacing={0.5}
      aria-label="Окружение"
      value={value}
      onValueChange={(next) => {
        // Повторный клик по выбранному пункту даёт "" — окружение всегда выбрано.
        if (next === Environment.production || next === Environment.development) onChange(next);
      }}
    >
      {OPTIONS.map((env) => (
        <ToggleGroupItem key={env} value={env} className="px-3 font-mono text-label-sm">
          {env}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
