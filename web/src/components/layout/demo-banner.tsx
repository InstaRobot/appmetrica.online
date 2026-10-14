import { FlaskConical } from "lucide-react";

// Плашка демо-режима над шапкой дашборда. Выход — полная навигация на /demo/exit (route handler
// снимает cookie и уводит на лендинг), поэтому обычная ссылка, а не next/link.
export function DemoBanner() {
  return (
    <section aria-label="Демо-режим" className="border-b border-border bg-warning-50">
      <div className="mx-auto flex max-w-[1600px] items-center gap-3 px-8 py-2.5 text-body-sm">
        <FlaskConical aria-hidden="true" className="size-4 shrink-0 text-warning" />
        <p>Демо-режим: данные синтетические, изменения не сохраняются.</p>
        <a href="/demo/exit" className="ml-auto font-semibold text-fg-link hover:underline">
          Выйти из демо
        </a>
      </div>
    </section>
  );
}
