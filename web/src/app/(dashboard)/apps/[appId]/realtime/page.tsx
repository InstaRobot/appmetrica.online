import type { Metadata } from "next";
import { Suspense } from "react";
import { RealtimePanel } from "@/components/reports/realtime-panel";
import { RealtimeSkeleton } from "@/components/skeletons/realtime-skeleton";

export const metadata: Metadata = { title: "Realtime" };

// S-07 `/apps/[appId]/realtime`: окружение в URL (nuqs читает useSearchParams), поэтому панель — под Suspense.
export default async function RealtimePage({ params }: PageProps<"/apps/[appId]/realtime">) {
  const { appId } = await params;
  return (
    <Suspense fallback={<RealtimeSkeleton />}>
      <RealtimePanel appId={appId} />
    </Suspense>
  );
}
