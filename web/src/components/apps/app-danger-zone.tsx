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
import { useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
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
// На телефоне кнопка каждого действия стоит под его описанием и растянута на всю ширину.
export function AppDangerZone({ app }: { app: App }) {
  const m = useMessages();
  const t = m.apps.danger;
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
      toast.success(t.archived);
      setArchiving(false);
      void queryClient.invalidateQueries({ queryKey: getGetAppQueryKey(app.id) });
      refreshLists();
    },
    onError: (error) => toast.error(uiError(error).message),
  });

  const unarchive = useMutation({
    mutationFn: () => unarchiveApp(app.id),
    onSuccess: () => {
      toast.success(t.restored);
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
      toast.success(t.deleteStarted);
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
        <CardTitle>{t.title}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {busy ? <p className="text-body-sm text-fg-subtle">{t.mergeBusy}</p> : null}
        <div className="flex items-center justify-between gap-4 max-md:flex-col max-md:items-stretch max-md:gap-3">
          <div className="flex flex-col gap-1">
            <p className="text-body-md font-medium">{t.mergeTitle}</p>
            <p className="text-body-md">{t.mergeDescription}</p>
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
            {t.merge}
          </Button>
        </div>
        <Separator />
        <div className="flex items-center justify-between gap-4 max-md:flex-col max-md:items-stretch max-md:gap-3">
          <p className="text-body-md">{archived ? t.archivedText : t.archiveText}</p>
          {archived ? (
            <Button
              variant="outline"
              disabled={busy || unarchive.isPending}
              onClick={() => unarchive.mutate()}
            >
              {unarchive.isPending ? t.restoring : t.restore}
            </Button>
          ) : (
            <Button variant="outline" disabled={busy} onClick={() => setArchiving(true)}>
              {t.archive}
            </Button>
          )}
        </div>
        <Separator />
        <div className="flex items-center justify-between gap-4 max-md:flex-col max-md:items-stretch max-md:gap-3">
          <p className="text-body-md">{t.deleteText}</p>
          <Button
            variant="outline"
            className="text-destructive"
            disabled={busy}
            onClick={() => setDeleting(true)}
          >
            {t.delete}
          </Button>
        </div>
      </CardContent>

      <AlertDialog open={archiving} onOpenChange={setArchiving}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{fmt(t.archiveTitle, { name: app.name })}</AlertDialogTitle>
            <AlertDialogDescription>{t.archiveDescription}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{m.common.cancel}</AlertDialogCancel>
            <Button disabled={archive.isPending} onClick={() => archive.mutate()}>
              {archive.isPending ? t.archiving : t.archive}
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
        title={fmt(t.deleteTitle, { name: app.name })}
        description={t.deleteDescription}
        confirmText={app.name}
        confirmLabel={t.delete}
        pending={remove.isPending}
        {...(remove.isError ? { serverError: uiError(remove.error).message } : {})}
        onConfirm={confirmDelete}
      />
    </Card>
  );
}
