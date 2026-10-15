import { PlatformIcon, platformLabel } from "@/components/common/platform-icon";
import type { Platform } from "@/lib/api/gen/model";
import { cn } from "@/lib/utils";

type PlatformIconsProps = {
  platforms: (Platform | string)[];
  /** Сколько иконок показать; остальные сворачиваются в «+N». */
  max?: number;
  className?: string;
};

// Иконки платформ приложения: первые `max` и «+N», все названия — в title.
// Декоративны для скринридеров: рядом всегда есть название приложения или подпись платформ.
export function PlatformIcons({ platforms, max = 3, className }: PlatformIconsProps) {
  if (platforms.length === 0) return null;
  const shown = platforms.slice(0, max);
  const hidden = platforms.length - shown.length;
  return (
    <span
      aria-hidden="true"
      title={platforms.map(platformLabel).join(", ")}
      className={cn("inline-flex shrink-0 items-center gap-0.5", className)}
    >
      {shown.map((platform) => (
        <PlatformIcon key={platform} platform={platform} className="size-4 shrink-0" />
      ))}
      {hidden > 0 ? <span className="text-label-sm tabular-nums">+{hidden}</span> : null}
    </span>
  );
}
