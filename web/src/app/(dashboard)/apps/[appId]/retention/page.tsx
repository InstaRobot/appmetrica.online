import type { Metadata } from "next";
import { Suspense } from "react";
import { RetentionSkeleton } from "@/components/skeletons/retention-skeleton";
import { RetentionView } from "./retention-view";

export const metadata: Metadata = { title: "Retention" };

// S-06 `/apps/[appId]/retention`: состояние отчёта в URL (nuqs читает useSearchParams), поэтому вид — под Suspense.
export default function RetentionPage() {
  return (
    <Suspense fallback={<RetentionSkeleton />}>
      <RetentionView />
    </Suspense>
  );
}
