import type { Metadata } from "next";
import { Suspense } from "react";
import { AudienceView } from "./audience-view";
import AudienceLoading from "./loading";

export const metadata: Metadata = { title: "Аудитория" };

// S-05 `/apps/[appId]`: состояние отчёта в URL (nuqs читает useSearchParams), поэтому вид — под Suspense.
export default function AudiencePage() {
  return (
    <Suspense fallback={<AudienceLoading />}>
      <AudienceView />
    </Suspense>
  );
}
