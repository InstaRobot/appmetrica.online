"use client";

import type { Route } from "next";
import Link from "next/link";
import { useState } from "react";
import { CodeBlock, type CodeBlockLang } from "@/components/common/code-block";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { Platform } from "@/lib/api/gen/model";
import {
  buildSnippet,
  defaultSnippetLang,
  SNIPPET_LABELS,
  SNIPPET_LANGS,
  SNIPPET_RECIPES,
  type SnippetLang,
} from "@/lib/snippets";

type SnippetTabsProps = {
  /** https://i.appmetrica.online/v1/events */
  ingestUrl: string;
  /** pk_… */
  writeKey: string;
  platform: Platform;
  /** По умолчанию — язык платформы (defaultSnippetLang). */
  defaultLang?: SnippetLang;
};

const CODE_LANG: Record<SnippetLang, CodeBlockLang> = {
  curl: "bash",
  swift: "swift",
  kotlin: "kotlin",
  javascript: "javascript",
  go: "go",
};

const isSnippetLang = (value: string): value is SnippetLang =>
  (SNIPPET_LANGS as readonly string[]).includes(value);

// SnippetTabs (FR-017, 14-ux-ui-spec.md §16.12): вкладки Radix (стрелки ←/→) со сниппетами,
// в которые подставлены write-ключ и эндпоинт ingest.
export function SnippetTabs({ ingestUrl, writeKey, platform, defaultLang }: SnippetTabsProps) {
  const [lang, setLang] = useState<SnippetLang>(defaultLang ?? defaultSnippetLang(platform));

  return (
    <Tabs value={lang} onValueChange={(value) => isSnippetLang(value) && setLang(value)}>
      <TabsList aria-label="Язык сниппета">
        {SNIPPET_LANGS.map((l) => (
          <TabsTrigger key={l} value={l}>
            {SNIPPET_LABELS[l]}
          </TabsTrigger>
        ))}
      </TabsList>
      {SNIPPET_LANGS.map((l) => (
        <TabsContent key={l} value={l}>
          <CodeBlock
            code={buildSnippet(l, { ingestUrl, writeKey, platform })}
            lang={CODE_LANG[l]}
            ariaLabel={`Пример кода на ${SNIPPET_LABELS[l]}`}
          />
        </TabsContent>
      ))}
      <Link
        href={SNIPPET_RECIPES[lang] as Route}
        className="text-body-sm text-fg-link hover:underline"
      >
        Полные рецепты с офлайн-очередью и сессиями — в документации →
      </Link>
    </Tabs>
  );
}
