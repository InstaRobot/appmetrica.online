"use client";

import { useDocumentLocale } from "@/components/feedback/use-document-locale";
import { systemError as en } from "@/messages/en/system-error";
import { systemError as ru } from "@/messages/ru/system-error";

type Props = { error: Error & { digest?: string }; reset: () => void };

// Вне MessagesProvider: язык — из <html lang> упавшей страницы, тексты — из короткого модуля system-error.
const TEXTS = { en, ru };

// Телефон: кнопка и ссылка — цели нажатия 44 px (подписи шире 44 px). Медиазапрос во встроенном <style>:
// стилями атрибута его не задать, а десктопный вид страницы остаётся прежним. Строка короткая — она
// попадает в initial JS каждой страницы.
const PHONE_CSS =
  "@media (width<48rem){.global-error-action{display:inline-flex;align-items:center;min-height:44px}}";

// Фатальная ошибка корневого layout: globals.css и шрифты могут быть недоступны, поэтому разметка
// минимальная со встроенными стилями (14-ux-ui-spec.md, Screen S-14).
export default function GlobalError({ reset }: Props) {
  const locale = useDocumentLocale();
  const m = TEXTS[locale];
  return (
    <html lang={locale}>
      <body style={{ fontFamily: "system-ui, sans-serif", margin: 0, color: "#1f2328" }}>
        <style>{PHONE_CSS}</style>
        <main
          style={{
            display: "flex",
            minHeight: "100vh",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            gap: 16,
            padding: "0 16px",
            textAlign: "center",
          }}
        >
          <h1 style={{ fontSize: 22, fontWeight: 600, margin: 0 }}>{m.title}</h1>
          <p style={{ margin: 0 }}>{m.text}</p>
          <div style={{ display: "flex", flexWrap: "wrap", justifyContent: "center", gap: 8 }}>
            <button type="button" className="global-error-action" onClick={reset}>
              {m.retry}
            </button>
            <a href="/portfolio" className="global-error-action">
              {m.toPortfolio}
            </a>
          </div>
        </main>
      </body>
    </html>
  );
}
