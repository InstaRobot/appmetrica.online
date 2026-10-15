import type { Metadata } from "next";
import { Suspense } from "react";
import { RealtimePanel } from "@/components/reports/realtime-panel";
import { RealtimeSkeleton } from "@/components/skeletons/realtime-skeleton";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";

// Заголовок вкладки на языке запроса: панель и так рендерится по запросу (язык и демо в layout).
export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).realtime.title };
}

// S-07 `/apps/[appId]/realtime`: окружение в URL (nuqs читает useSearchParams), поэтому панель — под Suspense.
export default async function RealtimePage({ params }: PageProps<"/apps/[appId]/realtime">) {
  const { appId } = await params;
  return (
    <Suspense fallback={<RealtimeSkeleton />}>
      <RealtimePanel appId={appId} />
    </Suspense>
  );
}
