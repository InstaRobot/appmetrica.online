"use client";

import { cn } from "cn";
import { useRef } from "react";
import { PLATFORM_LABELS } from "@/components/common/platform-icon";
import { toggleVariants } from "@/components/ui/toggle";
import { Platform } from "@/lib/api/gen/model";

const PLATFORMS = Object.values(Platform);

export type PlatformSwitcherProps = {
  /** Явные платформы приложения (app.platforms). */
  appPlatforms: Platform[];
  /** Наблюдаемые за период значения events.platform (breakdown dimension=platform). */
  observed: string[];
  /** Текущий фильтр из URL (?platform=), null — «Все». */
  value: Platform | null;
  onChange: (platform: Platform | null) => void;
};

// platformOptions — объединение явных, наблюдаемых и выбранной платформ в порядке enum; неизвестные значения
// events.platform фильтром не задаются. Выбранная остаётся в списке, чтобы её можно было сбросить.
export function platformOptions(
  appPlatforms: readonly Platform[],
  observed: readonly string[],
  value: Platform | null,
): Platform[] {
  const present = new Set<string>([...appPlatforms, ...observed]);
  if (value) present.add(value);
  return PLATFORMS.filter((p) => present.has(p));
}

// Переключатель платформ над отчётами (04-ux-diff.md §5.5): «Все» и платформы приложения. Radiogroup с
// roving tabindex: стрелки, Home и End выбирают опцию сразу, как у сегментированных контролов отчётов.
// Меньше двух платформ — переключатель не нужен.
export function PlatformSwitcher({
  appPlatforms,
  observed,
  value,
  onChange,
}: PlatformSwitcherProps) {
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const platforms = platformOptions(appPlatforms, observed, value);
  if (platforms.length < 2) return null;

  const options: (Platform | null)[] = [null, ...platforms];
  const current = Math.max(0, options.indexOf(value));

  const select = (index: number) => {
    const next = options[index] ?? null;
    refs.current[index]?.focus();
    if (next !== value) onChange(next);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const last = options.length - 1;
    const target = {
      ArrowRight: current === last ? 0 : current + 1,
      ArrowDown: current === last ? 0 : current + 1,
      ArrowLeft: current === 0 ? last : current - 1,
      ArrowUp: current === 0 ? last : current - 1,
      Home: 0,
      End: last,
    }[e.key];
    if (target === undefined) return;
    e.preventDefault();
    select(target);
  };

  return (
    <div
      role="radiogroup"
      aria-label="Платформа"
      onKeyDown={onKeyDown}
      className="flex w-fit items-center gap-0.5 rounded-[9px] bg-muted p-0.5"
    >
      {options.map((option, i) => {
        const checked = i === current;
        return (
          // biome-ignore lint/a11y/useSemanticElements: сегментированный контрол, как Radix ToggleGroup отчётов, а не поле формы
          <button
            key={option ?? "all"}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={checked ? 0 : -1}
            data-state={checked ? "on" : "off"}
            data-variant="segmented"
            onClick={() => select(i)}
            className={cn(toggleVariants({ variant: "segmented", size: "sm" }), "px-3")}
          >
            {option ? PLATFORM_LABELS[option] : "Все"}
          </button>
        );
      })}
    </div>
  );
}
