"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CircleAlert, GitMerge } from "lucide-react";
import { useEffect, useRef } from "react";
import { Alert, AlertAction, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  getListAppMergesQueryKey,
  retryAppMerge,
  useListAppMerges,
} from "@/lib/api/gen/endpoints/admin-apps/admin-apps";
import { getListWriteKeysQueryKey } from "@/lib/api/gen/endpoints/admin-keys/admin-keys";
import { getGetAppQueryKey, getListAppsQueryKey } from "@/lib/api/gen/endpoints/apps/apps";
import { getGetPortfolioQueryKey } from "@/lib/api/gen/endpoints/portfolio/portfolio";
import type { AppMerge } from "@/lib/api/gen/model";
import { formatInt } from "@/lib/format";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";

const POLL_MS = 5000;
const RUNNING = new Set<AppMerge["status"]>(["pending", "running"]);

// Слияния в приложение: опрос каждые 5 с, пока есть pending/running. Список новыми
// первыми; активным считается и failed — сервер блокирует архивацию, удаление и новое слияние до повтора.
function useAppMerges(appId: string) {
  const query = useListAppMerges(appId, {
    query: {
      refetchInterval: (q) =>
        q.state.data?.status === 200 && q.state.data.data.items.some((m) => RUNNING.has(m.status))
          ? POLL_MS
          : false,
    },
  });
  return query.data?.status === 200 ? query.data.data.items : [];
}

export function useActiveMerge(appId: string): AppMerge | undefined {
  return useAppMerges(appId).find((m) => RUNNING.has(m.status) || m.status === "failed");
}

function stepText(merge: AppMerge): string {
  const source = `Объединение с «${merge.source_name}»`;
  if (merge.status === "failed") {
    return `Объединение прервалось: ${merge.error ?? "причина неизвестна"}`;
  }
  if (merge.step === "wait_keys") {
    return `${source}: ждём, пока приём событий перечитает ключи (до 17 мин)`;
  }
  return `${source}: перенесено ${formatInt(merge.copied_events)} из ${formatInt(merge.total_events)} событий`;
}

// Баннер AppShell: шаг слияния текстом, «Повторить» у упавшего. Шаг move_keys
// переносит ключи источника и расширяет платформы цели — приложение и его write-ключи перечитываются, как
// только слияние прошло этот шаг. Когда отслеживаемое слияние завершилось, перечитываются данные цели
// (приложение, отчёты, ключи) и списки приложений.
export function MergeProgressBanner({ appId }: { appId: string }) {
  const uiError = useUiError();
  const queryClient = useQueryClient();
  const merges = useAppMerges(appId);
  const active = merges.find((m) => RUNNING.has(m.status) || m.status === "failed");
  const tracked = useRef<{ id: string; keysMoved: boolean } | null>(null);

  useEffect(() => {
    if (active) {
      const keysMoved = active.step !== "move_keys";
      if (tracked.current?.id === active.id && (tracked.current.keysMoved || !keysMoved)) return;
      tracked.current = { id: active.id, keysMoved };
      if (!keysMoved) return;
      void queryClient.invalidateQueries({ queryKey: getGetAppQueryKey(appId) });
      void queryClient.invalidateQueries({ queryKey: getListWriteKeysQueryKey(appId) });
      return;
    }
    const finished = merges.find((m) => m.id === tracked.current?.id && m.status === "done");
    if (!finished) return;
    tracked.current = null;
    toast.success("Приложения объединены");
    void queryClient.invalidateQueries({
      predicate: ({ queryKey }) =>
        typeof queryKey[0] === "string" && queryKey[0].startsWith(`/v1/apps/${appId}`),
    });
    void queryClient.invalidateQueries({ queryKey: getListWriteKeysQueryKey(appId) });
    void queryClient.invalidateQueries({ queryKey: getListAppsQueryKey() });
    void queryClient.invalidateQueries({ queryKey: getGetPortfolioQueryKey() });
  }, [active, merges, appId, queryClient]);

  const retry = useMutation({
    mutationFn: (mergeId: string) => retryAppMerge(appId, mergeId),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: getListAppMergesQueryKey(appId) }),
    onError: (error) => toast.error(uiError(error).message),
  });

  if (!active) return null;
  const failed = active.status === "failed";

  return (
    <Alert
      role="status"
      aria-live="polite"
      variant={failed ? "destructive" : "default"}
      className="mb-6"
    >
      {failed ? <CircleAlert aria-hidden="true" /> : <GitMerge aria-hidden="true" />}
      <AlertDescription className={failed ? undefined : "text-foreground"}>
        {stepText(active)}
      </AlertDescription>
      {failed ? (
        <AlertAction>
          <Button
            size="sm"
            variant="outline"
            disabled={retry.isPending}
            onClick={() => retry.mutate(active.id)}
          >
            Повторить
          </Button>
        </AlertAction>
      ) : null}
    </Alert>
  );
}
