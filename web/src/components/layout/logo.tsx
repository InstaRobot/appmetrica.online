// Wordmark (14-ux-ui-spec.md §1.3, макет «Dashboard v2»): знак 24×24 — градиентный квадрат с тремя
// столбиками 6/10/14 px — и текст «appmetrica.online». Градиент на CSS: без SVG-id, лого может
// повторяться на странице.
export function Logo() {
  return (
    <span className="inline-flex items-center gap-2.5">
      <span
        aria-hidden="true"
        className="flex size-6 items-end justify-center gap-0.5 rounded-[7px] bg-linear-135 from-(--primary-top) to-(--primary-hover) pb-[3px]"
      >
        <span className="h-1.5 w-[3px] rounded-[1px] bg-white" />
        <span className="h-2.5 w-[3px] rounded-[1px] bg-white" />
        <span className="h-3.5 w-[3px] rounded-[1px] bg-white" />
      </span>
      <span className="hidden text-lg font-bold text-foreground min-[400px]:inline">
        appmetrica<span className="text-fg-muted">.online</span>
      </span>
    </span>
  );
}
