"use client";

import { useDocumentLocale } from "@/components/feedback/use-document-locale";
import { systemError as en } from "@/messages/en/system-error";
import { systemError as ru } from "@/messages/ru/system-error";

type Props = { error: Error & { digest?: string }; reset: () => void };

// Вне MessagesProvider: язык — из <html lang> упавшей страницы, тексты — из короткого модуля system-error.
const TEXTS = { en, ru };

// Фатальная ошибка корневого layout: globals.css и шрифты могут быть недоступны, поэтому разметка
// минимальная со встроенными стилями (14-ux-ui-spec.md, Screen S-14).
export default function GlobalError({ reset }: Props) {
  const locale = useDocumentLocale();
  const m = TEXTS[locale];
  return (
    <html lang={locale}>
      <body style={{ fontFamily: "system-ui, sans-serif", margin: 0, color: "#1f2328" }}>
        <main
          style={{
            display: "flex",
            minHeight: "100vh",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            gap: 16,
            textAlign: "center",
          }}
        >
          <h1 style={{ fontSize: 22, fontWeight: 600, margin: 0 }}>{m.title}</h1>
          <p style={{ margin: 0 }}>{m.text}</p>
          <div style={{ display: "flex", gap: 8 }}>
            <button type="button" onClick={reset}>
              {m.retry}
            </button>
            <a href="/portfolio">{m.toPortfolio}</a>
          </div>
        </main>
      </body>
    </html>
  );
}
