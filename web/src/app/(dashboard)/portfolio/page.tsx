import type { Metadata } from "next";
import { Suspense } from "react";
import DashboardLoading from "../loading";
import { PortfolioView } from "../portfolio-view";

export const metadata: Metadata = { title: "Портфель" };

// S-03 `/portfolio` (на «/» — публичный лендинг S-00): состояние экрана в URL (nuqs читает useSearchParams), поэтому вид — под Suspense.
export default function PortfolioPage() {
  return (
    <Suspense fallback={<DashboardLoading />}>
      <PortfolioView />
    </Suspense>
  );
}
