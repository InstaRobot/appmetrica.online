import { DocsLayout } from "fumadocs-ui/layouts/docs";
import { RootProvider } from "fumadocs-ui/provider/next";
import { Suspense } from "react";
import { Providers } from "@/app/(dashboard)/providers";
import { AppHeader } from "@/components/layout/app-header";
import { requireOwner } from "@/lib/docs/session";
import { source } from "@/lib/docs/source";
import { DOCS_TRANSLATIONS } from "@/lib/docs/translations";
import "../docs.css";

// DocsShell (S-13): шапка дашборда сверху, под ней layout Fumadocs (сайдбар, контент, оглавление).
// Документация только для владельца: сессия проверяется на сервере при каждом запросе,
// 'use cache' в этой route group запрещён (AGENTS.md §5).
// Провайдер Fumadocs читает usePathname, а сессия — cookie запроса, поэтому вся оболочка под
// Suspense: статическая часть ответа — только полоса шапки.
export default function DocsShell({ children }: LayoutProps<"/docs">) {
  return (
    <Providers>
      <Suspense fallback={<div className="h-16 border-b border-border bg-header" />}>
        <RootProvider
          theme={{ enabled: false }}
          search={{ options: { api: "/docs/search" } }}
          i18n={{ locale: "ru", translations: DOCS_TRANSLATIONS }}
        >
          <AppHeader />
          <OwnerOnly>
            <DocsLayout
              tree={source.getPageTree()}
              nav={{ title: "Документация", url: "/docs" }}
              themeSwitch={{ enabled: false }}
              tabs={false}
            >
              {children}
            </DocsLayout>
          </OwnerOnly>
        </RootProvider>
      </Suspense>
    </Providers>
  );
}

async function OwnerOnly({ children }: { children: React.ReactNode }) {
  await requireOwner();
  return children;
}
