import type { Metadata } from "next";
import { AppSettingsPanel } from "@/components/apps/app-settings-panel";

export const metadata: Metadata = { title: "Настройки приложения" };

// S-11 `/apps/[appId]/settings`: имя, часовой пояс, write-ключи, архив и удаление (FR-009, FR-010).
export default async function AppSettingsPage({ params }: PageProps<"/apps/[appId]/settings">) {
  const { appId } = await params;
  return <AppSettingsPanel appId={appId} />;
}
