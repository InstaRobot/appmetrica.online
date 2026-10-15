import type { Metadata } from "next";
import { Suspense } from "react";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";
import DashboardLoading from "../loading";
import { PortfolioView } from "../portfolio-view";

// Заголовок вкладки на языке запроса: панель и так рендерится по запросу (язык и демо в layout).
export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).portfolio.title };
}

// S-03 `/portfolio` (на «/» — публичный лендинг S-00): состояние экрана в URL (nuqs читает useSearchParams), поэтому вид — под Suspense.
export default function PortfolioPage() {
  return (
    <Suspense fallback={<DashboardLoading />}>
      <PortfolioView />
    </Suspense>
  );
}
