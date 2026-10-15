"use client";

import type * as React from "react";
import { PLATFORM_LABELS, PlatformIcon } from "@/components/common/platform-icon";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Platform } from "@/lib/api/gen/model";

const PLATFORMS = Object.values(Platform);

type PlatformToggleGroupProps = {
  value: Platform[];
  onValueChange: (value: Platform[]) => void;
  disabled?: boolean;
  /** Ref первого чипа: форма ставит на него фокус, если набор пуст при отправке. */
  ref?: React.Ref<HTMLButtonElement>;
  "aria-label"?: string;
  "aria-labelledby"?: string;
  "aria-describedby"?: string | undefined;
};

// Мультивыбор платформ приложения: 9 чипов в порядке enum, у каждого иконка и
// подпись. Tab переходит между чипами, Space/Enter переключают (aria-pressed), поэтому вместо
// toolbar со стрелками у Radix — обычная group. Набор отдаётся в порядке enum независимо от порядка
// нажатий.
export function PlatformToggleGroup({
  value,
  onValueChange,
  disabled,
  ref,
  ...aria
}: PlatformToggleGroupProps) {
  return (
    <ToggleGroup
      type="multiple"
      role="group"
      variant="outline"
      rovingFocus={false}
      disabled={disabled}
      className="w-full flex-wrap"
      value={value}
      onValueChange={(next) => onValueChange(PLATFORMS.filter((p) => next.includes(p)))}
      {...aria}
    >
      {PLATFORMS.map((platform, index) => (
        <ToggleGroupItem
          key={platform}
          ref={index === 0 ? ref : undefined}
          value={platform}
          className="min-w-[104px] justify-start data-[state=on]:border-primary"
        >
          <PlatformIcon platform={platform} />
          {PLATFORM_LABELS[platform]}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
