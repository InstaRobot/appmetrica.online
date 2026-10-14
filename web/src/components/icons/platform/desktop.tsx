import { type PlatformGlyphProps, PlatformSvg } from "./platform-svg";

// Windows и Linux: монитор на подставке.
export function DesktopIcon(props: PlatformGlyphProps) {
  return (
    <PlatformSvg name="desktop" {...props}>
      <path
        fillRule="evenodd"
        d="M4 3h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2h-6v2h3v2H7v-2h3v-2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Zm0 2v10h16V5H4Z"
      />
    </PlatformSvg>
  );
}
