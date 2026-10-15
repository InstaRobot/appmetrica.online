import { AndroidIcon } from "@/components/icons/platform/android";
import { AppleIcon } from "@/components/icons/platform/apple";
import { BotIcon } from "@/components/icons/platform/bot";
import { DesktopIcon } from "@/components/icons/platform/desktop";
import { OtherIcon } from "@/components/icons/platform/other";
import type { PlatformGlyphProps } from "@/components/icons/platform/platform-svg";
import { ServerIcon } from "@/components/icons/platform/server";
import { WebIcon } from "@/components/icons/platform/web";
import type { Platform } from "@/lib/api/gen/model";
import { useMessages } from "@/lib/i18n/client";

export type PlatformLabels = Record<Platform, string>;

// Русские подписи для экранов, ещё не переведённых на словарь (common.platforms, usePlatformLabel);
// удаляются вместе с русскими обёртками lib/format.
export const PLATFORM_LABELS: PlatformLabels = {
  ios: "iOS",
  android: "Android",
  web: "Веб",
  macos: "macOS",
  windows: "Windows",
  linux: "Linux",
  bot: "Бот",
  server: "Бэкенд",
  other: "Другое",
};

// Цвет платформы на графиках: один и тот же в легенде, на полосе долей и в tooltip.
export const PLATFORM_CHART_VAR: Record<Platform, string> = {
  ios: "var(--chart-1)",
  macos: "var(--chart-1)",
  android: "var(--chart-3)",
  web: "var(--chart-2)",
  bot: "var(--chart-4)",
  server: "var(--chart-4)",
  windows: "var(--chart-5)",
  linux: "var(--chart-5)",
  other: "var(--chart-6)",
};

// Цвет значения events.platform: неизвестное — как «Другое».
export function platformChartVar(platform: string): string {
  return Object.hasOwn(PLATFORM_CHART_VAR, platform)
    ? PLATFORM_CHART_VAR[platform as Platform]
    : PLATFORM_CHART_VAR.other;
}

const GLYPHS: Record<Platform, (props: PlatformGlyphProps) => React.JSX.Element> = {
  ios: AppleIcon,
  macos: AppleIcon,
  android: AndroidIcon,
  web: WebIcon,
  windows: DesktopIcon,
  linux: DesktopIcon,
  bot: BotIcon,
  server: ServerIcon,
  other: OtherIcon,
};

// Подпись платформы устройства: известное значение — из подписей языка, неизвестное — как есть,
// пустое — «—».
export function platformLabelIn(labels: PlatformLabels, platform: string): string {
  return Object.hasOwn(labels, platform) ? labels[platform as Platform] : platform || "—";
}

// Русская подпись (PLATFORM_LABELS) для ещё не переведённых экранов.
export function platformLabel(platform: string): string {
  return platformLabelIn(PLATFORM_LABELS, platform);
}

// Подпись платформы на языке интерфейса (словарь common.platforms).
export function usePlatformLabel(): (platform: string) => string {
  const labels = useMessages().common.platforms;
  return (platform) => platformLabelIn(labels, platform);
}

type PlatformIconProps = { platform: Platform | string; className?: string; title?: string };

// Иконка платформы приложения (`app.platform`) или устройства (`context.platform`); неизвестное
// значение рисуется как «Другое».
export function PlatformIcon({ platform, className, title }: PlatformIconProps) {
  const Glyph = Object.hasOwn(GLYPHS, platform) ? GLYPHS[platform as Platform] : OtherIcon;
  return <Glyph className={className} title={title} />;
}
