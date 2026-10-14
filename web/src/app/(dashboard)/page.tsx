import type { Metadata } from "next";
import { PageHeader } from "@/components/layout/page-header";

export const metadata: Metadata = { title: "Портфель" };

// S-03: каркас страницы портфеля. Таблица PortfolioTable, дата и EnvironmentSwitch — T-037.
export default function PortfolioPage() {
  return <PageHeader title="Портфель" />;
}
