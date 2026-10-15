"use client";

import { CircleCheck, Loader2, TriangleAlert } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { useId } from "react";
import { CopyField } from "@/components/common/copy-field";
import { PlatformIcon } from "@/components/common/platform-icon";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card } from "@/components/ui/card";
import type { Platform, WriteKey } from "@/lib/api/gen/model";
import { useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { cn } from "@/lib/utils";
import { SnippetTabs } from "./snippet-tabs";

// activeWriteKeyFor — первый неотозванный ключ платформы по created_at asc, иначе первый неотозванный
// ключ без платформы (FR-7).
export function activeWriteKeyFor(keys: WriteKey[], platform: Platform): WriteKey | undefined {
  const active = keys
    .filter((k) => k.revoked_at === null)
    .sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at));
  return active.find((k) => k.platform === platform) ?? active.find((k) => k.platform === null);
}

type PlatformIntegrationSectionProps = {
  appId: string;
  platform: Platform;
  writeKey: WriteKey | undefined;
  /** Платформа наблюдалась за 48 ч. */
  received: boolean;
  ingestUrl: string;
};

// Секция платформы на вкладке «Интеграция»: статус первого события, write-ключ и сниппеты с этой
// платформой; без активного ключа — предупреждение со ссылкой на создание ключа.
export function PlatformIntegrationSection({
  appId,
  platform,
  writeKey,
  received,
  ingestUrl,
}: PlatformIntegrationSectionProps) {
  const platformLabels = useMessages().common.platforms;
  const m = useMessages().integration.platform;
  const titleId = useId();
  const label = platformLabels[platform];
  const vars = { platform: label };

  return (
    <section aria-labelledby={titleId}>
      <Card className="gap-3 px-4 md:px-6">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <PlatformIcon platform={platform} className="size-5 shrink-0" />
          <h3 id={titleId} className="text-heading-3">
            {label}
          </h3>
          <p
            role="status"
            aria-live="polite"
            aria-label={fmt(m.status, vars)}
            className={cn(
              "ml-auto flex items-center gap-1.5 text-body-sm",
              received ? "text-positive" : "text-fg-muted",
            )}
          >
            {received ? (
              <CircleCheck aria-hidden="true" className="size-4" />
            ) : (
              <Loader2 aria-hidden="true" className="size-4 animate-spin" />
            )}
            {received ? m.received : fmt(m.waiting, vars)}
          </p>
        </div>
        {writeKey ? (
          <>
            <p className="text-body-sm text-fg-muted">{m.writeKeyHint}</p>
            <CopyField value={writeKey.key} label={m.writeKeyField} />
            <SnippetTabs ingestUrl={ingestUrl} writeKey={writeKey.key} platform={platform} />
          </>
        ) : (
          <Alert className="border-warning/40">
            <TriangleAlert aria-hidden="true" className="text-warning" />
            <AlertDescription className="flex flex-wrap gap-x-2">
              <span>{fmt(m.noKey, vars)}</span>
              <Link
                href={`/apps/${appId}/settings#write-keys` as Route}
                className="text-fg-link hover:underline"
              >
                {fmt(m.createKey, vars)}
              </Link>
            </AlertDescription>
          </Alert>
        )}
      </Card>
    </section>
  );
}
