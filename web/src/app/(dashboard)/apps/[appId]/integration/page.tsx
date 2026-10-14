import type { Metadata } from "next";
import { IntegrationPanel } from "@/components/integration/integration-panel";

export const metadata: Metadata = { title: "Интеграция" };

// S-10 `/apps/[appId]/integration`: онбординг без SDK и здоровье приёма событий (FR-017).
export default async function IntegrationPage({ params }: PageProps<"/apps/[appId]/integration">) {
  const { appId } = await params;
  return <IntegrationPanel appId={appId} />;
}
