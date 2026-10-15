import type { Metadata } from "next";
import { Suspense } from "react";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";
import { SettingsHeader, SettingsView } from "./settings-view";

export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).settings.title };
}

// S-12 `/settings`: вкладка в `?tab=` (nuqs читает useSearchParams), поэтому вкладки — под Suspense.
export default function SettingsPage() {
  return (
    <>
      <SettingsHeader />
      <Suspense fallback={<TableSkeleton rows={3} />}>
        <SettingsView />
      </Suspense>
    </>
  );
}
