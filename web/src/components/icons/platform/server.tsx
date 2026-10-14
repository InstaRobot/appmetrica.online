import { type PlatformGlyphProps, PlatformSvg } from "./platform-svg";

// Бэкенд: две стойки сервера с индикаторами.
export function ServerIcon(props: PlatformGlyphProps) {
  return (
    <PlatformSvg name="server" {...props}>
      <path
        fillRule="evenodd"
        d="M5 3h14a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Zm1.5 3a1 1 0 1 0 0 2 1 1 0 0 0 0-2ZM5 13h14a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4a2 2 0 0 1 2-2Zm1.5 3a1 1 0 1 0 0 2 1 1 0 0 0 0-2Z"
      />
    </PlatformSvg>
  );
}
