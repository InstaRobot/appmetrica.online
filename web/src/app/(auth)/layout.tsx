import { Suspense } from "react";
import { MessagesProvider } from "@/lib/i18n/client";
import { HtmlLang } from "@/lib/i18n/html-lang";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";

// AuthShell (14-ux-ui-spec.md §5): центрированная карточка 400 px на --bg-page для S-01 и S-02.
// Заголовок h1 страницы имеет id="auth-title" — на него ссылается <main>. Язык читается из запроса,
// поэтому содержимое карточки — под Suspense.
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <main
      aria-labelledby="auth-title"
      className="flex min-h-dvh items-center justify-center bg-page px-4 py-12"
    >
      <div className="flex w-full max-w-[400px] flex-col gap-4 rounded-xl border border-border bg-card p-8 shadow-md">
        <p className="text-heading-3">
          appmetrica<span className="text-fg-subtle">.online</span>
        </p>
        <Suspense fallback={null}>
          <AuthMessages>{children}</AuthMessages>
        </Suspense>
      </div>
    </main>
  );
}

async function AuthMessages({ children }: { children: React.ReactNode }) {
  const locale = await getRequestLocale();
  return (
    <MessagesProvider locale={locale} messages={getMessages(locale)}>
      <HtmlLang locale={locale} />
      {children}
    </MessagesProvider>
  );
}
