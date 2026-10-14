"use client";

import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { ConfirmDeleteDialog } from "@/components/feedback/confirm-delete-dialog";
import { createDeletionRequest } from "@/lib/api/gen/endpoints/deletion/deletion";
import type { DeletionRequest } from "@/lib/api/gen/model";
import { toUiError } from "@/lib/errors";
import { deletionSubject, formatPersonKey, type PersonKey } from "@/lib/person-key";

type ProfileDeleteDialogProps = {
  appId: string;
  appName: string;
  personKey: PersonKey;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Ответ 202: новый запрос или уже существующий pending/running по тому же person. */
  onCreated: (request: DeletionRequest) => void;
};

// Удаление данных человека по GDPR (14-ux-ui-spec.md S-09, FR-019): подтверждение вводом «УДАЛИТЬ»,
// затем createDeletionRequest (`u:` → user, `i:` → install). Ошибка API остаётся в диалоге.
export function ProfileDeleteDialog({
  appId,
  appName,
  personKey,
  open,
  onOpenChange,
  onCreated,
}: ProfileDeleteDialogProps) {
  const create = useMutation({
    mutationFn: () => createDeletionRequest(appId, deletionSubject(personKey)),
  });

  const confirm = async () => {
    try {
      const res = await create.mutateAsync();
      if (res.status !== 202) return;
      toast.success("Запрос на удаление создан");
      onCreated(res.data);
      onOpenChange(false);
    } catch {
      // Текст ошибки показывает serverError диалога.
    }
  };

  return (
    <ConfirmDeleteDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) create.reset();
        onOpenChange(next);
      }}
      title="Удалить все данные этого человека?"
      description={
        <>
          Будут удалены все события и связки устройств для{" "}
          <span className="font-mono break-all">{formatPersonKey(personKey)}</span> в приложении «
          {appName}». Агрегаты закрытых дней не меняются. Удаление выполняется в фоне до 24 часов и
          необратимо.
        </>
      }
      confirmText="УДАЛИТЬ"
      confirmLabel="Удалить данные"
      pending={create.isPending}
      {...(create.isError ? { serverError: toUiError(create.error).message } : {})}
      onConfirm={confirm}
    />
  );
}
