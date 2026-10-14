import { type PlatformGlyphProps, PlatformSvg } from "./platform-svg";

// Android: голова робота с антеннами (собственный рисунок).
export function AndroidIcon(props: PlatformGlyphProps) {
  return (
    <PlatformSvg name="android" {...props}>
      <path d="m6.3 5.1 1.6 2.5A9.4 9.4 0 0 0 2.5 16h19a9.4 9.4 0 0 0-5.4-8.4l1.6-2.5a.6.6 0 1 0-1-.6l-1.7 2.6a9.6 9.6 0 0 0-6 0L7.3 4.5a.6.6 0 1 0-1 .6ZM8 11a1.1 1.1 0 1 1 0 2.2A1.1 1.1 0 0 1 8 11Zm8 0a1.1 1.1 0 1 1 0 2.2 1.1 1.1 0 0 1 0-2.2Z" />
      <path d="M2.5 17.5h19V19a1.5 1.5 0 0 1-1.5 1.5H4A1.5 1.5 0 0 1 2.5 19v-1.5Z" />
    </PlatformSvg>
  );
}
