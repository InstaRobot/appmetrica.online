import type { Metadata } from "next";
import { AppSettingsPanel } from "@/components/apps/app-settings-panel";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";

export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).apps.settings.title };
}

// S-11 `/apps/[appId]/settings`: имя, часовой пояс, write-ключи, архив и удаление (FR-009, FR-010).
export default async function AppSettingsPage({ params }: PageProps<"/apps/[appId]/settings">) {
  const { appId } = await params;
  return <AppSettingsPanel appId={appId} />;
}
