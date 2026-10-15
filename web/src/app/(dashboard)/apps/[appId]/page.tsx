import type { Metadata } from "next";
import { Suspense } from "react";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";
import { AudienceView } from "./audience-view";
import AudienceLoading from "./loading";

// Заголовок вкладки на языке запроса: панель и так рендерится по запросу (язык и демо в layout).
export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).reports.audience.title };
}

// S-05 `/apps/[appId]`: состояние отчёта в URL (nuqs читает useSearchParams), поэтому вид — под Suspense.
export default function AudiencePage() {
  return (
    <Suspense fallback={<AudienceLoading />}>
      <AudienceView />
    </Suspense>
  );
}
