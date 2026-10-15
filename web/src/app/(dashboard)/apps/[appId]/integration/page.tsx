import type { Metadata } from "next";
import { IntegrationPanel } from "@/components/integration/integration-panel";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";

export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).integration.title };
}

// S-10 `/apps/[appId]/integration`: онбординг без SDK и здоровье приёма событий (FR-017).
export default async function IntegrationPage({ params }: PageProps<"/apps/[appId]/integration">) {
  const { appId } = await params;
  return <IntegrationPanel appId={appId} />;
}
