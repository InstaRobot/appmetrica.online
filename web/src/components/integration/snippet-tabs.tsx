"use client";

import Link from "next/link";
import { useState } from "react";
import { CodeBlock, type CodeBlockLang } from "@/components/common/code-block";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { Platform } from "@/lib/api/gen/model";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { publicHref } from "@/lib/i18n/routes";
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
  const m = useMessages().integration;
  const locale = useLocale();

  return (
    <Tabs value={lang} onValueChange={(value) => isSnippetLang(value) && setLang(value)}>
      <TabsList aria-label={m.snippets.tabs}>
        {SNIPPET_LANGS.map((l) => (
          <TabsTrigger key={l} value={l}>
            {SNIPPET_LABELS[l]}
          </TabsTrigger>
        ))}
      </TabsList>
      {SNIPPET_LANGS.map((l) => (
        <TabsContent key={l} value={l}>
          <CodeBlock
            code={buildSnippet(l, { ingestUrl, writeKey, platform }, m.snippetComments)}
            lang={CODE_LANG[l]}
            ariaLabel={fmt(m.snippets.code, { lang: SNIPPET_LABELS[l] })}
          />
        </TabsContent>
      ))}
      <Link
        href={publicHref(locale, SNIPPET_RECIPES[lang])}
        className="text-body-sm text-fg-link hover:underline"
      >
        {m.snippets.recipes}
      </Link>
    </Tabs>
  );
}
