import { type PlatformGlyphProps, PlatformSvg } from "./platform-svg";

// iOS и macOS: силуэт яблока с листом (собственный рисунок, не логотип Apple).
export function AppleIcon(props: PlatformGlyphProps) {
  return (
    <PlatformSvg name="apple" {...props}>
      <path d="M15.6 2.5c.2 1.4-.4 2.8-1.3 3.7-.9.9-2.1 1.4-3.2 1.3-.2-1.3.4-2.7 1.3-3.6.9-.9 2.2-1.4 3.2-1.4Z" />
      <path d="M19.8 16.9c-.5 1.2-.8 1.7-1.5 2.7-1 1.4-2.3 3.1-4 3.1-1.5 0-1.9-1-3.9-1s-2.5 1-4 1c-1.6 0-2.9-1.6-3.9-3C.3 15.6 0 11.1 1.6 8.8 2.7 7.1 4.4 6.2 6 6.2c1.7 0 2.7 1 4.1 1 1.3 0 2.1-1 4.1-1 1.4 0 2.9.8 4 2.1-3.5 1.9-2.9 6.9 1.6 8.6Z" />
    </PlatformSvg>
  );
}
