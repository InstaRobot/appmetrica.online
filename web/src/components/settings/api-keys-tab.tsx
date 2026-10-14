"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { KeySquare, Plus, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as zod from "zod";
import { CopyField } from "@/components/common/copy-field";
import { FieldError, Required } from "@/components/common/form-field";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
  createApiKey,
  getListApiKeysQueryKey,
  revokeApiKey,
  useListApiKeys,
} from "@/lib/api/gen/endpoints/admin-keys/admin-keys";
import { useListApps } from "@/lib/api/gen/endpoints/apps/apps";
import { type ApiKey, ApiKeyScope, type AppSummary } from "@/lib/api/gen/model";
import { CreateApiKeyBody } from "@/lib/api/gen/zod/admin-keys/admin-keys.zod";
import { toUiError } from "@/lib/errors";
import { formatDate, formatDateTime, shortId } from "@/lib/format";
import { useZodForm } from "@/lib/forms/use-zod-form";
import { TabHeader } from "./tab-header";

const ALL_APPS = "all";
const SCOPES = Object.values(ApiKeyScope);

// F-4 (14-ux-ui-spec.md §7.1): метка 1–64 после trim, хотя бы одно право; «Все приложения» — без app_id.
const schema = zod.object({
  label: zod.string().trim().pipe(CreateApiKeyBody.shape.label),
  scopes: CreateApiKeyBody.shape.scopes,
  app_id: zod.string(),
});

const messages = {
  label: { too_small: "Введите метку", too_big: "Не длиннее 64 символов" },
  scopes: "Выберите хотя бы одно право",
};

const keyLabel = (key: ApiKey) => `${key.key_prefix}…`;

function useAppNames(): { apps: AppSummary[]; nameOf: (id: string) => string } {
  const query = useListApps({ include_archived: true });
  const apps = query.data?.status === 200 ? query.data.data.items : [];
  return {
    apps,
    nameOf: (id) => apps.find((a) => a.id === id)?.name ?? shortId(id),
  };
}

function CreateApiKeyForm({
  apps,
  onSecret,
}: {
  apps: AppSummary[];
  onSecret: (s: string) => void;
}) {
  const queryClient = useQueryClient();
  const form = useZodForm(schema, {
    defaultValues: { label: "", scopes: [ApiKeyScope.read], app_id: ALL_APPS },
    messages,
    onSubmit: async ({ label, scopes, app_id }) => {
      try {
        const res = await createApiKey({
          label,
          scopes,
          ...(app_id !== ALL_APPS ? { app_id } : {}),
        });
        if (res.status !== 201) return;
        onSecret(res.data.secret);
        void queryClient.invalidateQueries({ queryKey: getListApiKeysQueryKey() });
      } catch (error) {
        if (!form.applyServerErrors(error)) toast.error(toUiError(error).message);
      }
    },
  });

  const label = form.field("label");
  const scopes = form.field("scopes");
  const app = form.field("app_id");
  const toggleScope = (scope: ApiKeyScope, checked: boolean) => {
    const next = new Set(form.values.scopes);
    if (checked) next.add(scope);
    else next.delete(scope);
    form.setValue(
      "scopes",
      SCOPES.filter((s) => next.has(s)),
    );
  };

  return (
    <form noValidate onSubmit={form.handleSubmit} className="grid gap-4">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={label.id}>
          Метка <Required />
        </Label>
        <Input
          {...label}
          autoComplete="off"
          aria-required="true"
          placeholder="Например, Telegram-отчёт"
          value={form.values.label}
          onChange={(e) => form.setValue("label", e.target.value)}
          onBlur={() => form.blur("label")}
        />
        <FieldError id={form.errorId("label")} message={form.errors.label} />
      </div>

      <fieldset
        className="flex flex-col gap-1.5"
        aria-invalid={scopes["aria-invalid"]}
        aria-describedby={scopes["aria-describedby"]}
      >
        <legend className="mb-1.5 text-label-md">
          Права <Required />
        </legend>
        <div className="flex gap-4">
          {SCOPES.map((scope, i) => (
            <label key={scope} className="flex items-center gap-2 text-body-md">
              <input
                type="checkbox"
                ref={i === 0 ? scopes.ref : undefined}
                className="size-4 accent-primary"
                checked={form.values.scopes.includes(scope)}
                onChange={(e) => toggleScope(scope, e.target.checked)}
              />
              <span className="font-mono">{scope}</span>
            </label>
          ))}
        </div>
        <FieldError id={form.errorId("scopes")} message={form.errors.scopes} />
      </fieldset>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor={app.id}>Приложение</Label>
        <Select value={form.values.app_id} onValueChange={(v) => form.setValue("app_id", v)}>
          <SelectTrigger id={app.id} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_APPS}>Все приложения</SelectItem>
            {apps.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <DialogFooter>
        <DialogClose asChild>
          <Button type="button" variant="outline">
            Отмена
          </Button>
        </DialogClose>
        <Button type="submit" disabled={form.submitting}>
          {form.submitting ? "Создаём…" : "Создать"}
        </Button>
      </DialogFooter>
    </form>
  );
}

// Диалог создания API-ключа: после createApiKey секрет показывается один раз, закрыть диалог можно только
// кнопкой «Я сохранил ключ» (Esc и клик мимо не работают). Секрет живёт только в состоянии диалога.
function CreateApiKeyDialog({
  open,
  onOpenChange,
  apps,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  apps: AppSummary[];
}) {
  const [secret, setSecret] = useState<string | null>(null);
  const close = () => {
    setSecret(null);
    onOpenChange(false);
  };
  const keepWhileSecret = (e: Event) => {
    if (secret) e.preventDefault();
  };

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? onOpenChange(true) : close())}>
      <DialogContent
        showCloseButton={!secret}
        onEscapeKeyDown={keepWhileSecret}
        onInteractOutside={keepWhileSecret}
      >
        <DialogHeader>
          <DialogTitle>Новый API-ключ</DialogTitle>
          <DialogDescription>
            Ключ для Read API: заголовок Authorization: Bearer &lt;секрет&gt;.
          </DialogDescription>
        </DialogHeader>
        {secret ? (
          <>
            <CopyField value={secret} label="секрет" />
            <Alert className="border-warning/40 text-warning">
              <TriangleAlert aria-hidden="true" />
              <AlertDescription className="text-warning">
                Секрет показывается один раз. Сохраните его сейчас — восстановить нельзя.
              </AlertDescription>
            </Alert>
            <DialogFooter>
              <Button onClick={close}>Я сохранил ключ</Button>
            </DialogFooter>
          </>
        ) : (
          <CreateApiKeyForm apps={apps} onSecret={setSecret} />
        )}
      </DialogContent>
    </Dialog>
  );
}

function RevokeApiKeyDialog({ apiKey, onClose }: { apiKey: ApiKey | null; onClose: () => void }) {
  const queryClient = useQueryClient();
  const revoke = useMutation({
    mutationFn: (id: string) => revokeApiKey(id),
    onSuccess: () => {
      toast.success("Ключ отозван");
      onClose();
      void queryClient.invalidateQueries({ queryKey: getListApiKeysQueryKey() });
    },
    onError: (error) => toast.error(toUiError(error).message),
  });

  return (
    <AlertDialog open={apiKey !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Отозвать ключ {apiKey ? keyLabel(apiKey) : ""}?</AlertDialogTitle>
          <AlertDialogDescription>
            Запросы к Read API с этим ключом сразу начнут получать 401.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Отмена</AlertDialogCancel>
          <Button
            variant="destructive"
            disabled={revoke.isPending}
            onClick={() => apiKey && revoke.mutate(apiKey.id)}
          >
            {revoke.isPending ? "Отзываем…" : "Отозвать"}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// Вкладка «API-ключи» S-12 (FR-011): ключи `sk_` для Read API. В таблице только key_prefix — секрет
// бэкенд не хранит и показывает один раз при создании.
export function ApiKeysTab({ timezone }: { timezone: string }) {
  const query = useListApiKeys();
  const { apps, nameOf } = useAppNames();
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<ApiKey | null>(null);
  const keys = query.data?.status === 200 ? query.data.data.items : undefined;

  return (
    <section aria-labelledby="api-keys-title" className="flex flex-col gap-4">
      <TabHeader
        id="api-keys-title"
        title="API-ключи"
        action={
          keys && keys.length > 0 ? (
            <Button size="sm" onClick={() => setCreating(true)}>
              <Plus aria-hidden="true" />
              Создать ключ
            </Button>
          ) : null
        }
      />
      {query.isError && !keys ? (
        <ErrorState
          error={query.error as unknown as ApiError}
          onRetry={() => void query.refetch()}
        />
      ) : !keys ? (
        <TableSkeleton rows={2} />
      ) : keys.length === 0 ? (
        <EmptyState
          icon={KeySquare}
          title="API-ключей нет"
          description="Ключ нужен для скриптов и интеграций с Read API."
          action={{ label: "Создать ключ", onClick: () => setCreating(true) }}
        />
      ) : (
        <Table>
          <caption className="sr-only">API-ключи</caption>
          <TableHeader>
            <TableRow>
              <TableHead>Ключ</TableHead>
              <TableHead>Метка</TableHead>
              <TableHead>Права</TableHead>
              <TableHead>Приложение</TableHead>
              <TableHead>Создан</TableHead>
              <TableHead>Использован</TableHead>
              <TableHead>
                <span className="sr-only">Действия</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map((key) => (
              <TableRow key={key.id}>
                <TableCell>
                  <span className="flex items-center gap-2">
                    <span className="font-mono text-code">{keyLabel(key)}</span>
                    {key.revoked_at ? <Badge variant="secondary">Отозван</Badge> : null}
                  </span>
                </TableCell>
                <TableCell>{key.label}</TableCell>
                <TableCell className="font-mono">{key.scopes.join(", ")}</TableCell>
                <TableCell>{key.app_id ? nameOf(key.app_id) : "Все"}</TableCell>
                <TableCell>{formatDate(key.created_at, timezone)}</TableCell>
                <TableCell>
                  {key.last_used_at ? formatDateTime(key.last_used_at, timezone) : "—"}
                </TableCell>
                <TableCell className="text-right">
                  {key.revoked_at ? null : (
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label={`Отозвать ключ ${keyLabel(key)}`}
                      onClick={() => setRevoking(key)}
                    >
                      Отозвать
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <CreateApiKeyDialog open={creating} onOpenChange={setCreating} apps={apps} />
      <RevokeApiKeyDialog apiKey={revoking} onClose={() => setRevoking(null)} />
    </section>
  );
}
