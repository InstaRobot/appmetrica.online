"use client";

import { useMutation } from "@tanstack/react-query";
import { ConfirmDeleteDialog } from "@/components/feedback/confirm-delete-dialog";
import { createDeletionRequest } from "@/lib/api/gen/endpoints/deletion/deletion";
import type { DeletionRequest } from "@/lib/api/gen/model";
import { useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { deletionSubject, formatPersonKey, type PersonKey } from "@/lib/person-key";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";

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
  const t = useMessages().profiles.deleteDialog;
  const uiError = useUiError();
  const create = useMutation({
    mutationFn: () => createDeletionRequest(appId, deletionSubject(personKey)),
  });

  const confirm = async () => {
    try {
      const res = await create.mutateAsync();
      if (res.status !== 202) return;
      toast.success(t.created);
      onCreated(res.data);
      onOpenChange(false);
    } catch {
      // Текст ошибки показывает serverError диалога.
    }
  };

  // person_key в описании — моноширинный: текст словаря делится по плейсхолдеру {key}.
  const [before = "", after = ""] = t.description
    .split("{key}")
    .map((part) => fmt(part, { app: appName }));

  return (
    <ConfirmDeleteDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) create.reset();
        onOpenChange(next);
      }}
      title={t.title}
      description={
        <>
          {before}
          <span className="font-mono break-all">{formatPersonKey(personKey)}</span>
          {after}
        </>
      }
      confirmText={t.confirmText}
      confirmLabel={t.confirm}
      pending={create.isPending}
      {...(create.isError ? { serverError: uiError(create.error).message } : {})}
      onConfirm={confirm}
    />
  );
}
