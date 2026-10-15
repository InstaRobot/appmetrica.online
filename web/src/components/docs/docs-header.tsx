import { AppHeader } from "@/components/layout/app-header";
import { PublicHeader } from "@/components/layout/public-header";
import { isOwner } from "@/lib/docs/session";

// Шапка документации (S-13): владельцу — шапка дашборда, гостю — публичная.
// Обёртка задаёт поведение над layout Fumadocs: с md шапка прилипает сверху, а на телефоне уезжает при
// прокрутке — сверху остаётся полоса Fumadocs с поиском и меню (docs.css обнуляет --fd-banner-height).
// z-35 — над прилипающими полосами Fumadocs (z-30), но под его выдвижным сайдбаром (z-40) и поиском
// (z-50); z-index самой шапки действует только внутри обёртки.
export async function DocsHeader() {
  return (
    <div className="relative z-35 md:sticky md:top-0">
      {(await isOwner()) ? (
        <AppHeader />
      ) : (
        <PublicHeader className="border-b border-border bg-header backdrop-blur-md" />
      )}
    </div>
  );
}
