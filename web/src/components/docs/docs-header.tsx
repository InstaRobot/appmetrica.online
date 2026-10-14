import { AppHeader } from "@/components/layout/app-header";
import { PublicHeader } from "@/components/layout/public-header";
import { isOwner } from "@/lib/docs/session";

// Шапка документации (S-13): владельцу — шапка дашборда, гостю — публичная.
export async function DocsHeader() {
  return (await isOwner()) ? (
    <AppHeader />
  ) : (
    <PublicHeader className="sticky top-0 z-sticky border-b border-border bg-header backdrop-blur-md" />
  );
}
