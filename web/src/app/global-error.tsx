"use client";

type Props = { error: Error & { digest?: string }; reset: () => void };

// Фатальная ошибка корневого layout: globals.css и шрифты могут быть недоступны, поэтому разметка
// минимальная со встроенными стилями (14-ux-ui-spec.md, Screen S-14).
export default function GlobalError({ reset }: Props) {
  return (
    <html lang="ru">
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
          <h1 style={{ fontSize: 22, fontWeight: 600, margin: 0 }}>Что-то пошло не так</h1>
          <p style={{ margin: 0 }}>
            Произошла непредвиденная ошибка. Попробуйте обновить страницу.
          </p>
          <div style={{ display: "flex", gap: 8 }}>
            <button type="button" onClick={reset}>
              Попробовать снова
            </button>
            <a href="/portfolio">В портфель</a>
          </div>
        </main>
      </body>
    </html>
  );
}
