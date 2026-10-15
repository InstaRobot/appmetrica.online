"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { GitMerge } from "lucide-react";
import { useRouter } from "next/navigation";
import { lazy, Suspense, useState } from "react";
import { ConfirmDeleteDialog } from "@/components/feedback/confirm-delete-dialog";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { archiveApp, deleteApp, unarchiveApp } from "@/lib/api/gen/endpoints/admin-apps/admin-apps";
import { getGetAppQueryKey, getListAppsQueryKey } from "@/lib/api/gen/endpoints/apps/apps";
import { getGetPortfolioQueryKey } from "@/lib/api/gen/endpoints/portfolio/portfolio";
import type { App } from "@/lib/api/gen/model";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";
import { useActiveMerge } from "./merge-progress-banner";

// Диалог слияния — отдельный чанк: нужен только после клика «Объединить…» (бюджет initial JS).
const MergeAppsDialog = lazy(() =>
  import("./merge-apps-dialog").then((m) => ({ default: m.MergeAppsDialog })),
);

// «Опасная зона» S-11 (FR-009): архивирование (ingest отвечает 403 app_archived), возврат из архива и
// удаление навсегда вводом точного названия. Удаление выполняет worker, экран сразу уходит в портфель.
// Слияние другого приложения в это; пока оно не завершено, опасные действия недоступны.
export function AppDangerZone({ app }: { app: App }) {
  const uiError = useUiError();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [archiving, setArchiving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [merging, setMerging] = useState(false);
  const [mergeRequested, setMergeRequested] = useState(false);
  const busy = useActiveMerge(app.id) !== undefined;

  const refreshLists = () => {
    void queryClient.invalidateQueries({ queryKey: getListAppsQueryKey() });
    void queryClient.invalidateQueries({ queryKey: getGetPortfolioQueryKey() });
  };

  const archive = useMutation({
    mutationFn: () => archiveApp(app.id),
    onSuccess: () => {
      toast.success("Приложение в архиве");
      setArchiving(false);
      void queryClient.invalidateQueries({ queryKey: getGetAppQueryKey(app.id) });
      refreshLists();
    },
    onError: (error) => toast.error(uiError(error).message),
  });

  const unarchive = useMutation({
    mutationFn: () => unarchiveApp(app.id),
    onSuccess: () => {
      toast.success("Приложение возвращено из архива");
      void queryClient.invalidateQueries({ queryKey: getGetAppQueryKey(app.id) });
      refreshLists();
    },
    onError: (error) => toast.error(uiError(error).message),
  });

  const remove = useMutation({ mutationFn: () => deleteApp(app.id, { confirm_name: app.name }) });

  const confirmDelete = async () => {
    try {
      const res = await remove.mutateAsync();
      if (res.status !== 202) return;
      toast.success("Удаление запущено");
      refreshLists();
      router.push("/portfolio");
    } catch {
      // Текст ошибки показывает serverError диалога.
    }
  };

  const archived = app.status === "archived";

  return (
    <Card className="border-destructive">
      <CardHeader>
        <CardTitle>Опасная зона</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {busy ? (
          <p className="text-body-sm text-fg-subtle">Идёт объединение — дождитесь завершения</p>
        ) : null}
        <div className="flex items-center justify-between gap-4">
          <div className="flex flex-col gap-1">
            <p className="text-body-md font-medium">Объединить с другим приложением</p>
            <p className="text-body-md">
              История событий и ключи другого приложения переедут сюда, а оно само будет удалено.
            </p>
          </div>
          <Button
            variant="outline"
            disabled={busy || archived}
            onClick={() => {
              setMergeRequested(true);
              setMerging(true);
            }}
          >
            <GitMerge aria-hidden="true" />
            Объединить…
          </Button>
        </div>
        <Separator />
        <div className="flex items-center justify-between gap-4">
          <p className="text-body-md">
            {archived
              ? "Приложение в архиве: события отклоняются, данные сохранены. Верните его, чтобы снова принимать события."
              : "Архивировать — приём событий прекращается (403 app_archived), данные сохраняются."}
          </p>
          {archived ? (
            <Button
              variant="outline"
              disabled={busy || unarchive.isPending}
              onClick={() => unarchive.mutate()}
            >
              {unarchive.isPending ? "Возвращаем…" : "Вернуть из архива"}
            </Button>
          ) : (
            <Button variant="outline" disabled={busy} onClick={() => setArchiving(true)}>
              Архивировать
            </Button>
          )}
        </div>
        <Separator />
        <div className="flex items-center justify-between gap-4">
          <p className="text-body-md">
            Удалить навсегда — все события и настройки удаляются в фоне.
          </p>
          <Button
            variant="outline"
            className="text-destructive"
            disabled={busy}
            onClick={() => setDeleting(true)}
          >
            Удалить навсегда
          </Button>
        </div>
      </CardContent>

      <AlertDialog open={archiving} onOpenChange={setArchiving}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Архивировать «{app.name}»?</AlertDialogTitle>
            <AlertDialogDescription>
              Ingest начнёт отклонять события этого приложения с кодом 403 app_archived. Данные и
              отчёты сохранятся, приложение можно вернуть из архива.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Отмена</AlertDialogCancel>
            <Button disabled={archive.isPending} onClick={() => archive.mutate()}>
              {archive.isPending ? "Архивируем…" : "Архивировать"}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {mergeRequested ? (
        <Suspense fallback={null}>
          <MergeAppsDialog target={app} open={merging} onOpenChange={setMerging} />
        </Suspense>
      ) : null}

      <ConfirmDeleteDialog
        open={deleting}
        onOpenChange={(open) => {
          if (!open) remove.reset();
          setDeleting(open);
        }}
        title={`Удалить «${app.name}» навсегда?`}
        description="Все события, ключи и настройки приложения будут удалены в фоне. Действие необратимо."
        confirmText={app.name}
        confirmLabel="Удалить навсегда"
        pending={remove.isPending}
        {...(remove.isError ? { serverError: uiError(remove.error).message } : {})}
        onConfirm={confirmDelete}
      />
    </Card>
  );
}
