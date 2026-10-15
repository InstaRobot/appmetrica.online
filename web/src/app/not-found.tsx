import { FileQuestion } from "lucide-react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { LOCALES, type Locale } from "@/lib/i18n/locale";
import { getMessages } from "@/messages";

// Страница статическая и стоит вне MessagesProvider: язык известен только браузеру. Поэтому в разметке
// оба языка, а виден тот, что inline-скрипт записал в <html lang> до первой отрисовки
// (lib/i18n/lang-script.ts): без JS и без мигания.
const SHOWN_FOR: Record<Locale, string> = {
  en: "[html[lang=ru]_&]:hidden",
  ru: "[html:not([lang=ru])_&]:hidden",
};

// S-14a: глобальная 404 (14-ux-ui-spec.md §5, Screen S-14).
export default function NotFound() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-4 px-4 text-center md:px-8">
      <FileQuestion aria-hidden="true" className="size-12 text-fg-subtle" />
      {LOCALES.map((locale) => {
        const m = getMessages(locale).system.notFound;
        return (
          <div
            key={locale}
            lang={locale}
            className={`flex flex-col items-center gap-4 ${SHOWN_FOR[locale]}`}
          >
            <h1 className="text-heading-1">{m.title}</h1>
            <p className="text-body-md text-muted-foreground">{m.text}</p>
            {/* Телефон: цель нажатия 44 px, с md — обычная кнопка 36 px. */}
            <Button asChild className="h-11 md:h-9">
              <Link href="/portfolio">{m.action}</Link>
            </Button>
          </div>
        );
      })}
    </main>
  );
}
