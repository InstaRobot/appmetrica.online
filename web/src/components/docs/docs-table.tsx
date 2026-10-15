import type { ComponentProps } from "react";

// Таблица MDX: та же обёртка с прокруткой, что у Fumadocs, но в порядке табуляции и с видимым фокусом — на
// телефоне широкая таблица прокручивается вбок, и без фокуса её не прокрутить с клавиатуры (WCAG 2.1.1, axe
// scrollable-region-focusable).
export function DocsTable(props: ComponentProps<"table">) {
  return (
    <div
      // biome-ignore lint/a11y/noNoninteractiveTabindex: прокручиваемая область должна получать фокус с клавиатуры
      tabIndex={0}
      className="relative my-6 overflow-auto rounded-md outline-none prose-no-margin focus-visible:ring-3 focus-visible:ring-ring/50"
    >
      <table {...props} />
    </div>
  );
}
