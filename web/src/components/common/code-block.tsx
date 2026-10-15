"use client";

import { Copy } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { useCopyText } from "@/lib/clipboard";
import { useMessages } from "@/lib/i18n/client";

export type CodeBlockLang = "bash" | "swift" | "kotlin" | "javascript" | "go" | "json";

type CodeBlockProps = {
  code: string;
  lang: CodeBlockLang;
  ariaLabel: string;
  maxHeight?: number;
};

// Shiki возвращает собственный <pre>; внутри CodeBlock нужен только <code> со строками.
const components = { pre: ({ children }: { children?: ReactNode }) => children };

// CodeBlock (14-ux-ui-spec.md §16.13): <pre tabIndex={0}> с прокруткой и кнопкой копирования.
// Подсветка shiki из fumadocs-core грузится лениво: до неё код показывается без цвета. Тема
// github-light-default на белом фоне держит контраст AA (axe color-contrast на S-10).
export function CodeBlock({ code, lang, ariaLabel, maxHeight = 480 }: CodeBlockProps) {
  const m = useMessages().common.copy;
  const copy = useCopyText();
  const [highlighted, setHighlighted] = useState<{ code: string; node: ReactNode } | null>(null);

  useEffect(() => {
    let active = true;
    import("fumadocs-core/highlight")
      .then(({ highlight }) => highlight(code, { lang, theme: "github-light-default", components }))
      .then((node) => {
        if (active) setHighlighted({ code, node });
      })
      .catch(() => {
        // Без подсветки код остаётся читаемым.
      });
    return () => {
      active = false;
    };
  }, [code, lang]);

  return (
    <div className="relative">
      {/* biome-ignore lint/a11y/useSemanticElements: <pre> сохраняет пробелы кода; region даёт области имя */}
      <pre
        // biome-ignore lint/a11y/noNoninteractiveTabindex: прокручиваемая область должна быть доступна с клавиатуры
        tabIndex={0}
        role="region"
        aria-label={ariaLabel}
        style={{ maxHeight }}
        className="overflow-auto rounded-md border border-border bg-card p-4 pr-14 text-code outline-none focus-visible:ring-2 focus-visible:ring-ring md:pr-12"
      >
        {highlighted?.code === code ? highlighted.node : <code>{code}</code>}
      </pre>
      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        aria-label={m.code}
        // Телефон: цель нажатия 44 px, с md — компактная кнопка 32 px.
        className="absolute top-2 right-2 size-11 bg-card md:size-8"
        onClick={() => void copy(code)}
      >
        <Copy aria-hidden="true" />
      </Button>
    </div>
  );
}
