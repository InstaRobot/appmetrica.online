// Wordmark (14-ux-ui-spec.md §1.3): знак 20×20 с тремя столбиками и текст «appmetrica.online».
export function Logo() {
  return (
    <span className="inline-flex items-center gap-2">
      <svg aria-hidden="true" width="20" height="20" viewBox="0 0 20 20" className="text-primary">
        <rect width="20" height="20" rx="5" fill="currentColor" />
        <rect x="3.5" y="11" width="3" height="6" fill="white" />
        <rect x="8.5" y="7" width="3" height="10" fill="white" />
        <rect x="13.5" y="3" width="3" height="14" fill="white" />
      </svg>
      <span className="text-lg font-semibold text-foreground">
        appmetrica<span className="text-fg-muted">.online</span>
      </span>
    </span>
  );
}
