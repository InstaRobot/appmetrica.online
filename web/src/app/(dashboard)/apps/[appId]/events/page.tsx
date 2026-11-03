import type { Metadata } from "next";
import { Suspense } from "react";
import { EventsView } from "@/components/events/events-view";
import { EventsSkeleton } from "@/components/skeletons/events-skeleton";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";

// Заголовок вкладки на языке запроса: панель и так рендерится по запросу (язык и демо в layout).
export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).events.title };
}

// `/apps/[appId]/events`: состояние списка в URL (nuqs читает useSearchParams), поэтому вид — под Suspense.
export default function EventsPage() {
  return (
    <Suspense fallback={<EventsSkeleton />}>
      <EventsView />
    </Suspense>
  );
}
