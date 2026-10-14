// «Перейти к содержимому» (WCAG 2.4.1): первый фокусируемый элемент страницы, виден только в фокусе.
export function SkipLink() {
  return (
    <a
      href="#main"
      className="sr-only rounded-md bg-card px-3 py-2 text-label-md text-fg-link shadow-md focus:not-sr-only focus:fixed focus:top-2 focus:left-2 z-skiplink"
    >
      Перейти к содержимому
    </a>
  );
}
