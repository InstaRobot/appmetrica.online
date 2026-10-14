import { type PlatformGlyphProps, PlatformSvg } from "./platform-svg";

// Прочие платформы: четыре квадрата.
export function OtherIcon(props: PlatformGlyphProps) {
  return (
    <PlatformSvg name="other" {...props}>
      <path d="M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z" />
    </PlatformSvg>
  );
}
