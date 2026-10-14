"use client";

import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { AppDangerZone } from "./app-danger-zone";
import { AppSettingsForm } from "./app-settings-form";
import { WriteKeysTable } from "./write-keys-table";

// S-11 `/apps/[appId]/settings`: колонка 720 px. Приложение уже загружено AppShell (тот же кэш запросов),
// ошибки и загрузку показывает он.
export function AppSettingsPanel({ appId }: { appId: string }) {
  const query = useGetApp(appId);
  const app = query.data?.status === 200 ? query.data.data : undefined;
  if (!app) return null;

  return (
    <div className="flex max-w-[720px] flex-col gap-6">
      <AppSettingsForm key={app.id} app={app} />
      <WriteKeysTable appId={app.id} platforms={app.platforms} timezone={app.timezone} />
      <AppDangerZone app={app} />
    </div>
  );
}
