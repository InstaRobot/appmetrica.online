"use client";

import { FileQuestion } from "lucide-react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { publicHref } from "@/lib/i18n/routes";

// S-13 not-found: неизвестная страница документации, внутри layout доков (сайдбар остаётся). not-found не
// получает params, поэтому язык и словарь — из MessagesProvider layout [lang].
export default function DocsNotFound() {
  const locale = useLocale();
  const m = useMessages().system;
  return (
    <main
      id="nd-page"
      className="flex flex-col items-center justify-center gap-4 px-8 py-24 text-center [grid-area:main]"
    >
      <FileQuestion aria-hidden="true" className="size-12 text-fg-subtle" />
      <h1 className="text-heading-1">{m.notFound.title}</h1>
      <p className="text-body-md text-muted-foreground">{m.docsNotFound.text}</p>
      <Button asChild>
        <Link href={publicHref(locale, "/docs")}>{m.docsNotFound.action}</Link>
      </Button>
    </main>
  );
}
