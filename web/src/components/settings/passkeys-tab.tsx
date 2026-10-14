"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { useId, useState } from "react";
import { FieldError } from "@/components/common/form-field";
import { ErrorState } from "@/components/feedback/error-state";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
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
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ApiError } from "@/lib/api/fetcher";
import {
  deletePasskey,
  getListPasskeysQueryKey,
  renamePasskey,
  useListPasskeys,
} from "@/lib/api/gen/endpoints/admin-passkeys/admin-passkeys";
import type { Passkey } from "@/lib/api/gen/model";
import { toUiError } from "@/lib/errors";
import { formatDate, formatDateTime } from "@/lib/format";
import { toast } from "@/lib/toast";
import { passkeyErrorMessage, registerAdditionalPasskey, validateDeviceName } from "@/lib/webauthn";
import { TabHeader } from "./tab-header";

type NameFormProps = {
  initialName: string;
  submitLabel: string;
  pendingLabel: string;
  /** Возвращает текст ошибки для диалога или null при успехе. */
  onSubmit: (name: string) => Promise<string | null>;
};

// F-6: название устройства 1–64 символа после trim; ошибка запроса остаётся в диалоге.
function PasskeyNameForm({ initialName, submitLabel, pendingLabel, onSubmit }: NameFormProps) {
  const id = useId();
  const [name, setName] = useState(initialName);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const invalid = validateDeviceName(name);
    setError(invalid);
    if (invalid) return;
    setPending(true);
    try {
      setError(await onSubmit(name.trim()));
    } finally {
      setPending(false);
    }
  };

  return (
    <form noValidate onSubmit={submit} className="grid gap-4">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={id}>Название устройства</Label>
        <Input
          id={id}
          autoComplete="off"
          placeholder="Например, iPhone"
          value={name}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? `${id}-error` : undefined}
          onChange={(e) => setName(e.target.value)}
        />
        <FieldError id={`${id}-error`} message={error ?? undefined} />
      </div>
      <DialogFooter>
        <DialogClose asChild>
          <Button type="button" variant="outline">
            Отмена
          </Button>
        </DialogClose>
        <Button type="submit" disabled={pending}>
          {pending ? pendingLabel : submitLabel}
        </Button>
      </DialogFooter>
    </form>
  );
}

type DeleteDialogProps = { passkey: Passkey | null; onClose: () => void };

function DeletePasskeyDialog({ passkey, onClose }: DeleteDialogProps) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: (id: string) => deletePasskey(id),
    onSuccess: () => {
      toast.success("Passkey удалён");
      onClose();
      void queryClient.invalidateQueries({ queryKey: getListPasskeysQueryKey() });
    },
  });

  return (
    <AlertDialog
      open={passkey !== null}
      onOpenChange={(open) => {
        if (open) return;
        remove.reset();
        onClose();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Удалить passkey «{passkey?.name}»?</AlertDialogTitle>
          <AlertDialogDescription>
            Войти с этого устройства или ключа больше не получится.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {remove.isError ? (
          <p role="alert" className="text-body-sm text-destructive">
            {toUiError(remove.error).message}
          </p>
        ) : null}
        <AlertDialogFooter>
          <AlertDialogCancel>Отмена</AlertDialogCancel>
          <Button
            variant="destructive"
            disabled={remove.isPending}
            onClick={() => passkey && remove.mutate(passkey.id)}
          >
            {remove.isPending ? "Удаляем…" : "Удалить"}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// Вкладка «Passkeys» S-12 (FR-008): несколько passkey владельца, добавление, переименование и удаление.
// Последний passkey удалить нельзя (409 last_passkey) — текст ошибки остаётся в диалоге.
export function PasskeysTab({ timezone }: { timezone: string }) {
  const queryClient = useQueryClient();
  const query = useListPasskeys();
  const [adding, setAdding] = useState(false);
  const [renaming, setRenaming] = useState<Passkey | null>(null);
  const [deleting, setDeleting] = useState<Passkey | null>(null);
  const passkeys = query.data?.status === 200 ? query.data.data.items : undefined;

  const refresh = () => queryClient.invalidateQueries({ queryKey: getListPasskeysQueryKey() });

  const add = async (name: string) => {
    try {
      await registerAdditionalPasskey(name);
    } catch (error) {
      return passkeyErrorMessage(error, "setup");
    }
    toast.success("Passkey добавлен");
    setAdding(false);
    await refresh();
    return null;
  };

  const rename = async (name: string) => {
    if (!renaming) return null;
    try {
      await renamePasskey(renaming.id, { name });
    } catch (error) {
      return toUiError(error).message;
    }
    setRenaming(null);
    await refresh();
    return null;
  };

  return (
    <section aria-labelledby="passkeys-title" className="flex flex-col gap-4">
      <TabHeader
        id="passkeys-title"
        title="Passkeys"
        action={
          <Button size="sm" onClick={() => setAdding(true)}>
            <Plus aria-hidden="true" />
            Добавить passkey
          </Button>
        }
      />
      {query.isError && !passkeys ? (
        <ErrorState
          error={query.error as unknown as ApiError}
          onRetry={() => void query.refetch()}
        />
      ) : !passkeys ? (
        <TableSkeleton rows={2} />
      ) : (
        <Table>
          <caption className="sr-only">Passkeys</caption>
          <TableHeader>
            <TableRow>
              <TableHead>Название</TableHead>
              <TableHead>Создан</TableHead>
              <TableHead>Последний вход</TableHead>
              <TableHead>Синхронизируется</TableHead>
              <TableHead>
                <span className="sr-only">Действия</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {passkeys.map((p) => (
              <TableRow key={p.id}>
                <TableCell className="font-medium">{p.name}</TableCell>
                <TableCell>{formatDate(p.created_at, timezone)}</TableCell>
                <TableCell>
                  {p.last_used_at ? formatDateTime(p.last_used_at, timezone) : "—"}
                </TableCell>
                <TableCell>{p.backup_state ? "да" : "нет"}</TableCell>
                <TableCell className="text-right">
                  <span className="inline-flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label={`Переименовать ${p.name}`}
                      onClick={() => setRenaming(p)}
                    >
                      Переименовать
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label={`Удалить ${p.name}`}
                      onClick={() => setDeleting(p)}
                    >
                      Удалить
                    </Button>
                  </span>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <Dialog open={adding} onOpenChange={setAdding}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Новый passkey</DialogTitle>
            <DialogDescription>
              Браузер предложит создать passkey на этом устройстве, в телефоне или на ключе.
            </DialogDescription>
          </DialogHeader>
          <PasskeyNameForm
            initialName=""
            submitLabel="Создать passkey"
            pendingLabel="Ждём подтверждения…"
            onSubmit={add}
          />
        </DialogContent>
      </Dialog>

      <Dialog open={renaming !== null} onOpenChange={(open) => !open && setRenaming(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Переименовать passkey</DialogTitle>
            <DialogDescription>Название видно только в этом списке.</DialogDescription>
          </DialogHeader>
          {renaming ? (
            <PasskeyNameForm
              initialName={renaming.name}
              submitLabel="Сохранить"
              pendingLabel="Сохраняем…"
              onSubmit={rename}
            />
          ) : null}
        </DialogContent>
      </Dialog>

      <DeletePasskeyDialog passkey={deleting} onClose={() => setDeleting(null)} />
    </section>
  );
}
