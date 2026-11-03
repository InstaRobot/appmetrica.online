import type { Metadata } from "next";
import { Suspense } from "react";
import { EventView } from "@/components/events/event-view";
import { EventSkeleton } from "@/components/skeletons/event-skeleton";

// Заголовок вкладки — имя события как отправлено (не переводится).
export async function generateMetadata({
  params,
}: PageProps<"/apps/[appId]/events/[eventName]">): Promise<Metadata> {
  const { eventName } = await params;
  return { title: eventName };
}

// `/apps/[appId]/events/[eventName]`: состояние отчёта в URL (nuqs читает useSearchParams), поэтому вид — под Suspense.
export default function EventPage() {
  return (
    <Suspense fallback={<EventSkeleton />}>
      <EventView />
    </Suspense>
  );
}
