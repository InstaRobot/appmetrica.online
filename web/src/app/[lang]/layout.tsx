import { notFound } from "next/navigation";
import { MessagesProvider } from "@/lib/i18n/client";
import { HtmlLang } from "@/lib/i18n/html-lang";
import { isLocale, LOCALES } from "@/lib/i18n/locale";
import { getMessages } from "@/messages";

// Публичные страницы на двух языках: лендинг и документация. EN приходит сюда как /en/… после rewrite в
// proxy.ts, RU — как /ru/…. Обе версии собираются статически; другой сегмент — 404 (dynamicParams
// несовместим с cacheComponents).
export function generateStaticParams() {
  return LOCALES.map((lang) => ({ lang }));
}

export default async function LocaleLayout({ children, params }: LayoutProps<"/[lang]">) {
  const { lang } = await params;
  if (!isLocale(lang)) notFound();
  return (
    <MessagesProvider locale={lang} messages={getMessages(lang)}>
      <HtmlLang locale={lang} />
      {children}
    </MessagesProvider>
  );
}
