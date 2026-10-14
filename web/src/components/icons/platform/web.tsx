import { type PlatformGlyphProps, PlatformSvg } from "./platform-svg";

// Веб: глобус с меридианами.
export function WebIcon(props: PlatformGlyphProps) {
  return (
    <PlatformSvg name="web" {...props}>
      <path
        fillRule="evenodd"
        d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20Zm-1.2 2.1C9.6 5.6 8.8 8 8.6 11H4.1a8 8 0 0 1 6.7-6.9Zm2.4 0A8 8 0 0 1 19.9 11h-4.5c-.2-3-1-5.4-2.2-6.9ZM10.6 11c.2-3 1.1-5.2 1.4-5.7.3.5 1.2 2.7 1.4 5.7h-2.8Zm-6.5 2h4.5c.2 3 1 5.4 2.2 6.9A8 8 0 0 1 4.1 13Zm6.5 0h2.8c-.2 3-1.1 5.2-1.4 5.7-.3-.5-1.2-2.7-1.4-5.7Zm4.8 0h4.5a8 8 0 0 1-6.7 6.9c1.2-1.5 2-3.9 2.2-6.9Z"
      />
    </PlatformSvg>
  );
}
