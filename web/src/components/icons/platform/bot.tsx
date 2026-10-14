import { type PlatformGlyphProps, PlatformSvg } from "./platform-svg";

// Бот: голова робота с антенной и глазами-прорезями.
export function BotIcon(props: PlatformGlyphProps) {
  return (
    <PlatformSvg name="bot" {...props}>
      <path
        fillRule="evenodd"
        d="M12 1.5a1.5 1.5 0 0 1 1 2.6V6h4a3 3 0 0 1 3 3v9a3 3 0 0 1-3 3H7a3 3 0 0 1-3-3V9a3 3 0 0 1 3-3h4V4.1a1.5 1.5 0 0 1 1-2.6ZM9 11a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Zm6 0a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Zm-6 5.5v1.5h6v-1.5H9Z"
      />
      <path d="M1 11h2v5H1zM21 11h2v5h-2z" />
    </PlatformSvg>
  );
}
