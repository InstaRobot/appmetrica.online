import type { Metadata } from "next";
import { Suspense } from "react";
import { RetentionSkeleton } from "@/components/skeletons/retention-skeleton";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";
import { RetentionView } from "./retention-view";

// Заголовок вкладки на языке запроса: панель и так рендерится по запросу (язык и демо в layout).
export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).retention.title };
}

// S-06 `/apps/[appId]/retention`: состояние отчёта в URL (nuqs читает useSearchParams), поэтому вид — под Suspense.
export default function RetentionPage() {
  return (
    <Suspense fallback={<RetentionSkeleton />}>
      <RetentionView />
    </Suspense>
  );
}
