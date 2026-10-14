import { Suspense } from "react";
import { AppFooter } from "@/components/layout/app-footer";
import { AppHeader } from "@/components/layout/app-header";
import { SkipLink } from "@/components/layout/skip-link";
import { Providers } from "./providers";

// DashboardShell (14-ux-ui-spec.md §5): SkipLink → AppHeader → <main id="main"> → AppFooter.
// AppHeader читает query-параметры отчётов (useSearchParams), поэтому стоит под Suspense:
// при cacheComponents статическая оболочка содержит только полосу шапки.
export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return (
    <Providers>
      <SkipLink />
      <Suspense fallback={<div className="h-16 border-b border-border bg-header" />}>
        <AppHeader />
      </Suspense>
      <main id="main" tabIndex={-1} className="mx-auto max-w-[1600px] px-8 pt-6 pb-12 outline-none">
        {children}
      </main>
      <AppFooter />
    </Providers>
  );
}
