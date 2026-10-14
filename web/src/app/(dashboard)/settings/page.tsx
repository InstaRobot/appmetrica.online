import type { Metadata } from "next";
import { Suspense } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { SettingsView } from "./settings-view";

export const metadata: Metadata = { title: "Настройки" };

// S-12 `/settings`: вкладка в `?tab=` (nuqs читает useSearchParams), поэтому вкладки — под Suspense.
export default function SettingsPage() {
  return (
    <>
      <PageHeader title="Настройки" />
      <Suspense fallback={<TableSkeleton rows={3} />}>
        <SettingsView />
      </Suspense>
    </>
  );
}
