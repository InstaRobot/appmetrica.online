"use client";

import { TriangleAlert } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { CopyField } from "@/components/common/copy-field";
import { ErrorState } from "@/components/feedback/error-state";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import type { ApiError } from "@/lib/api/fetcher";
import { useListWriteKeys } from "@/lib/api/gen/endpoints/admin-keys/admin-keys";
import type { App, WriteKey } from "@/lib/api/gen/model";
import { SnippetTabs } from "./snippet-tabs";

type IntegrationOnboardingProps = { app: App; ingestUrl: string };

// activeWriteKey — первый неотозванный ключ по created_at asc (14-ux-ui-spec.md S-10).
export function activeWriteKey(keys: WriteKey[]): WriteKey | undefined {
  return keys
    .filter((k) => k.revoked_at === null)
    .sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at))[0];
}

// Онбординг-блоки S-10: «1. Ключ приложения» и «2. Отправьте первое событие» со сниппетами.
export function IntegrationOnboarding({ app, ingestUrl }: IntegrationOnboardingProps) {
  const keys = useListWriteKeys(app.id);
  const list = keys.data?.status === 200 ? keys.data.data.items : undefined;

  if (!list) {
    if (keys.isError) {
      // apiFetch бросает ApiError; тип ошибки orval описывает тело ответа, а не исключение.
      return (
        <Card className="px-6">
          <ErrorState
            error={keys.error as unknown as ApiError}
            onRetry={() => void keys.refetch()}
          />
        </Card>
      );
    }
    return (
      <Card role="status" aria-label="Загрузка…" className="gap-3 px-6">
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-8 w-full" />
      </Card>
    );
  }

  const key = activeWriteKey(list);
  if (!key) {
    return (
      <Alert className="border-warning/40">
        <TriangleAlert aria-hidden="true" className="text-warning" />
        <AlertDescription className="flex flex-wrap gap-x-2">
          <span>Нет активного write-ключа. Создайте ключ в настройках приложения.</span>
          <Link href={`/apps/${app.id}/settings` as Route} className="text-fg-link hover:underline">
            Настройки приложения
          </Link>
        </AlertDescription>
      </Alert>
    );
  }

  return (
    <>
      <Card className="gap-3 px-6">
        <h3 className="text-heading-3">1. Ключ приложения</h3>
        <p className="text-body-sm text-fg-muted">
          Write-ключ (публичный, встраивается в приложение)
        </p>
        <CopyField value={key.key} label="write-ключ" />
        <p className="text-body-sm text-fg-muted">Эндпоинт</p>
        <CopyField value={ingestUrl} label="эндпоинт" />
      </Card>
      <Card className="gap-3 px-6">
        <h3 className="text-heading-3">2. Отправьте первое событие</h3>
        <SnippetTabs ingestUrl={ingestUrl} writeKey={key.key} platform={app.platform} />
      </Card>
    </>
  );
}
