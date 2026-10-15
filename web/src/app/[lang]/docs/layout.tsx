import { DocsLayout } from "fumadocs-ui/layouts/docs";
import { RootProvider } from "fumadocs-ui/provider/next";
import { Suspense } from "react";
import { Providers } from "@/app/(dashboard)/providers";
import { DocsHeader } from "@/components/docs/docs-header";
import { source } from "@/lib/docs/source";
import { DOCS_TRANSLATIONS } from "@/lib/docs/translations";
import "../docs.css";

// DocsShell (S-13): шапка сверху, под ней layout Fumadocs (сайдбар, контент, оглавление).
// Документация публичная (решение владельца от 2026-10-14); шапка зависит от сессии: владельцу —
// шапка дашборда, гостю — публичная. Сессию читаем из cookie запроса, поэтому 'use cache' в этой
// route group запрещён (CONTRIBUTING.md §5).
// Провайдер Fumadocs читает usePathname, а шапка — cookie запроса, поэтому вся оболочка под
// Suspense: статическая часть ответа — только полоса шапки.
export default function DocsShell({ children }: LayoutProps<"/[lang]/docs">) {
  return (
    <Providers>
      <Suspense fallback={<div className="h-16 border-b border-border bg-header" />}>
        <RootProvider
          theme={{ enabled: false }}
          search={{ options: { api: "/docs/search" } }}
          i18n={{ locale: "ru", translations: DOCS_TRANSLATIONS }}
        >
          <DocsHeader />
          <DocsLayout
            tree={source.getPageTree()}
            nav={{ title: "Документация", url: "/docs" }}
            themeSwitch={{ enabled: false }}
            tabs={false}
          >
            {children}
          </DocsLayout>
        </RootProvider>
      </Suspense>
    </Providers>
  );
}
