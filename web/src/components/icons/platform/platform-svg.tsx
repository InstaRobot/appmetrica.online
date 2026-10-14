// Общая оболочка иконок платформ (14-ux-ui-spec.md §11): viewBox 24×24, заливка currentColor.
// С title иконка — изображение с подписью, без title — декоративная (aria-hidden).
export type PlatformGlyphProps = { className?: string | undefined; title?: string | undefined };

export function PlatformSvg({
  name,
  className,
  title,
  children,
}: PlatformGlyphProps & { name: string; children: React.ReactNode }) {
  const props = {
    viewBox: "0 0 24 24",
    fill: "currentColor",
    className: className ?? "size-4",
    "data-platform-icon": name,
  };
  if (!title) {
    return (
      <svg aria-hidden="true" {...props}>
        {children}
      </svg>
    );
  }
  return (
    <svg role="img" aria-label={title} {...props}>
      <title>{title}</title>
      {children}
    </svg>
  );
}
