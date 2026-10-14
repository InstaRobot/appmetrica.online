import { AndroidIcon } from "@/components/icons/platform/android";
import { AppleIcon } from "@/components/icons/platform/apple";
import { BotIcon } from "@/components/icons/platform/bot";
import { DesktopIcon } from "@/components/icons/platform/desktop";
import { OtherIcon } from "@/components/icons/platform/other";
import type { PlatformGlyphProps } from "@/components/icons/platform/platform-svg";
import { ServerIcon } from "@/components/icons/platform/server";
import { WebIcon } from "@/components/icons/platform/web";
import type { Platform } from "@/lib/api/gen/model";

export const PLATFORM_LABELS: Record<Platform, string> = {
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

type PlatformIconProps = { platform: Platform | string; className?: string; title?: string };

// Иконка платформы приложения (`app.platform`) или устройства (`context.platform`); неизвестное
// значение рисуется как «Другое».
export function PlatformIcon({ platform, className, title }: PlatformIconProps) {
  const Glyph = Object.hasOwn(GLYPHS, platform) ? GLYPHS[platform as Platform] : OtherIcon;
  return <Glyph className={className} title={title} />;
}
