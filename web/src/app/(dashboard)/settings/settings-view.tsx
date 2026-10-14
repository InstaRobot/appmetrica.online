"use client";

import { parseAsStringLiteral, useQueryState } from "nuqs";
import { lazy, Suspense } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useGetSettings } from "@/lib/api/gen/endpoints/admin-settings/admin-settings";

// Radix Tabs монтирует только активную вкладку, поэтому код вкладки (формы, диалоги, WebAuthn) грузится
// отдельным чанком при первом открытии — в initial JS страницы его нет (бюджет NFR-006, T-044).
const PasskeysTab = lazy(() =>
  import("@/components/settings/passkeys-tab").then((m) => ({ default: m.PasskeysTab })),
);
const ApiKeysTab = lazy(() =>
  import("@/components/settings/api-keys-tab").then((m) => ({ default: m.ApiKeysTab })),
);
const TelegramTab = lazy(() =>
  import("@/components/settings/telegram-tab").then((m) => ({ default: m.TelegramTab })),
);
const SystemTab = lazy(() =>
  import("@/components/settings/system-tab").then((m) => ({ default: m.SystemTab })),
);
const DeletionsTab = lazy(() =>
  import("@/components/settings/deletions-tab").then((m) => ({ default: m.DeletionsTab })),
);

function TabSkeleton() {
  return (
    <div role="status" aria-label="Загрузка…" className="flex flex-col gap-4">
      <Skeleton className="h-8 w-64" />
      <Skeleton className="h-40 w-full" />
    </div>
  );
}

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
        <Suspense fallback={<TabSkeleton />}>
          <PasskeysTab timezone={timezone} />
        </Suspense>
      </TabsContent>
      <TabsContent value="api-keys">
        <Suspense fallback={<TabSkeleton />}>
          <ApiKeysTab timezone={timezone} />
        </Suspense>
      </TabsContent>
      <TabsContent value="telegram">
        <Suspense fallback={<TabSkeleton />}>
          <TelegramTab />
        </Suspense>
      </TabsContent>
      <TabsContent value="system">
        <Suspense fallback={<TabSkeleton />}>
          <SystemTab timezone={timezone} />
        </Suspense>
      </TabsContent>
      <TabsContent value="deletions">
        <Suspense fallback={<TabSkeleton />}>
          <DeletionsTab timezone={timezone} />
        </Suspense>
      </TabsContent>
    </Tabs>
  );
}
