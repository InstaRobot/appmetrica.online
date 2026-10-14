"use client";

import { parseAsStringLiteral, useQueryState } from "nuqs";
import { ApiKeysTab } from "@/components/settings/api-keys-tab";
import { DeletionsTab } from "@/components/settings/deletions-tab";
import { PasskeysTab } from "@/components/settings/passkeys-tab";
import { SystemTab } from "@/components/settings/system-tab";
import { TelegramTab } from "@/components/settings/telegram-tab";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useGetSettings } from "@/lib/api/gen/endpoints/admin-settings/admin-settings";

const TABS = [
  { value: "passkeys", label: "Passkeys" },
  { value: "api-keys", label: "API-ключи" },
  { value: "telegram", label: "Telegram" },
  { value: "system", label: "Система" },
  { value: "deletions", label: "Удаления" },
] as const;

type Tab = (typeof TABS)[number]["value"];

const tabParser = parseAsStringLiteral(TABS.map((t) => t.value)).withDefault("passkeys");

// S-12 `/settings`: вкладки синхронизированы с `?tab=` (неизвестное значение — Passkeys). Стрелки, Home и
// End переключают вкладки (Radix Tabs, автоматическая активация). Даты — в часовом поясе владельца.
export function SettingsView() {
  const [tab, setTab] = useQueryState("tab", tabParser);
  const settings = useGetSettings();
  const timezone =
    settings.data?.status === 200
      ? settings.data.data.owner_timezone
      : Intl.DateTimeFormat().resolvedOptions().timeZone;

  return (
    <Tabs value={tab} onValueChange={(value) => void setTab(value as Tab)} className="gap-6">
      <TabsList aria-label="Разделы настроек">
        {TABS.map((t) => (
          <TabsTrigger key={t.value} value={t.value}>
            {t.label}
          </TabsTrigger>
        ))}
      </TabsList>
      <TabsContent value="passkeys">
        <PasskeysTab timezone={timezone} />
      </TabsContent>
      <TabsContent value="api-keys">
        <ApiKeysTab timezone={timezone} />
      </TabsContent>
      <TabsContent value="telegram">
        <TelegramTab />
      </TabsContent>
      <TabsContent value="system">
        <SystemTab timezone={timezone} />
      </TabsContent>
      <TabsContent value="deletions">
        <DeletionsTab timezone={timezone} />
      </TabsContent>
    </Tabs>
  );
}
