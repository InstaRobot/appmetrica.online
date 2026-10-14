// Заглушка для красного теста T-036; реализация — в следующем коммите.
import type { Platform } from "@/lib/api/gen/model";
export const PLATFORM_LABELS: Record<Platform, string> = {
  ios: "",
  android: "",
  web: "",
  macos: "",
  windows: "",
  linux: "",
  bot: "",
  server: "",
  other: "",
};
export function PlatformIcon(_props: { platform: string; className?: string; title?: string }) {
  return null;
}
