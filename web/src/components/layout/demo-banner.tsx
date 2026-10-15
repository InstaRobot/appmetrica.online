import { FlaskConical } from "lucide-react";
import type { Locale } from "@/lib/i18n/locale";
import { getMessages } from "@/messages";

// Плашка демо-режима над шапкой дашборда. Выход — полная навигация на /demo/exit (route handler
// снимает cookie и уводит на лендинг), поэтому обычная ссылка, а не next/link.
export function DemoBanner({ locale }: { locale: Locale }) {
  const m = getMessages(locale).demo;
  return (
    <section aria-label={m.label} className="border-b border-border bg-warning-50">
      <div className="mx-auto flex max-w-[1600px] flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2.5 text-body-sm md:px-8">
        <p className="flex items-start gap-3">
          <FlaskConical aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-warning" />
          {m.notice}
        </p>
        <a href="/demo/exit" className="ml-auto font-semibold text-fg-link hover:underline">
          {m.exit}
        </a>
      </div>
    </section>
  );
}
