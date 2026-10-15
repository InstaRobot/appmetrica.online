"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { KeySquare, Plus, TriangleAlert } from "lucide-react";
import { useState } from "react";
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
import { useZodForm } from "@/lib/forms/use-zod-form";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";
import { TabHeader } from "./tab-header";

const ALL_APPS = "all";
const SCOPES = Object.values(ApiKeyScope);

// F-4 (14-ux-ui-spec.md §7.1): метка 1–64 после trim, хотя бы одно право; «Все приложения» — без app_id.
const schema = () => import("@/lib/forms/schemas").then((m) => m.apiKeySchema);

const keyLabel = (key: ApiKey) => `${key.key_prefix}…`;

function useAppNames(): { apps: AppSummary[]; nameOf: (id: string) => string } {
  const { shortId } = useFormat();
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
  const m = useMessages();
  const t = m.settings.apiKeys.form;
  const uiError = useUiError();
  const queryClient = useQueryClient();
  const form = useZodForm(schema, {
    defaultValues: { label: "", scopes: [ApiKeyScope.read], app_id: ALL_APPS },
    messages: {
      label: { too_small: t.labelRequired, too_big: t.labelTooLong },
      scopes: t.scopesRequired,
    },
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
        if (!form.applyServerErrors(error)) toast.error(uiError(error).message);
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
          {t.label} <Required />
        </Label>
        <Input
          {...label}
          autoComplete="off"
          aria-required="true"
          placeholder={t.labelPlaceholder}
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
          {t.scopes} <Required />
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
        <Label htmlFor={app.id}>{t.app}</Label>
        <Select value={form.values.app_id} onValueChange={(v) => form.setValue("app_id", v)}>
          <SelectTrigger id={app.id} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_APPS}>{t.allApps}</SelectItem>
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
            {m.common.cancel}
          </Button>
        </DialogClose>
        <Button type="submit" disabled={form.submitting}>
          {form.submitting ? t.submitting : t.submit}
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
  const t = useMessages().settings.apiKeys;
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
          <DialogTitle>{t.createTitle}</DialogTitle>
          <DialogDescription>{t.createDescription}</DialogDescription>
        </DialogHeader>
        {secret ? (
          <>
            <CopyField value={secret} label={t.secret} />
            <Alert className="border-warning/40 text-warning">
              <TriangleAlert aria-hidden="true" />
              <AlertDescription className="text-warning">{t.secretWarning}</AlertDescription>
            </Alert>
            <DialogFooter>
              <Button onClick={close}>{t.secretSaved}</Button>
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
  const m = useMessages();
  const t = m.settings.apiKeys;
  const uiError = useUiError();
  const queryClient = useQueryClient();
  const revoke = useMutation({
    mutationFn: (id: string) => revokeApiKey(id),
    onSuccess: () => {
      toast.success(t.revokedToast);
      onClose();
      void queryClient.invalidateQueries({ queryKey: getListApiKeysQueryKey() });
    },
    onError: (error) => toast.error(uiError(error).message),
  });

  return (
    <AlertDialog open={apiKey !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {fmt(t.revokeTitle, { key: apiKey ? keyLabel(apiKey) : "" })}
          </AlertDialogTitle>
          <AlertDialogDescription>{t.revokeDescription}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{m.common.cancel}</AlertDialogCancel>
          <Button
            variant="destructive"
            disabled={revoke.isPending}
            onClick={() => apiKey && revoke.mutate(apiKey.id)}
          >
            {revoke.isPending ? t.revoking : t.revoke}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// Вкладка «API-ключи» S-12 (FR-011): ключи `sk_` для Read API. В таблице только key_prefix — секрет
// бэкенд не хранит и показывает один раз при создании.
export function ApiKeysTab({ timezone }: { timezone: string }) {
  const m = useMessages();
  const t = m.settings.apiKeys;
  const f = useFormat();
  const query = useListApiKeys();
  const { apps, nameOf } = useAppNames();
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<ApiKey | null>(null);
  const keys = query.data?.status === 200 ? query.data.data.items : undefined;

  return (
    <section aria-labelledby="api-keys-title" className="flex flex-col gap-4">
      <TabHeader
        id="api-keys-title"
        title={t.title}
        action={
          keys && keys.length > 0 ? (
            <Button size="sm" onClick={() => setCreating(true)}>
              <Plus aria-hidden="true" />
              {t.create}
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
          title={t.emptyTitle}
          description={t.emptyDescription}
          action={{ label: t.create, onClick: () => setCreating(true) }}
        />
      ) : (
        <Table stackOnMobile>
          <caption className="sr-only">{t.title}</caption>
          <TableHeader>
            <TableRow>
              <TableHead>{t.columns.key}</TableHead>
              <TableHead>{t.columns.label}</TableHead>
              <TableHead>{t.columns.scopes}</TableHead>
              <TableHead>{t.columns.app}</TableHead>
              <TableHead>{t.columns.created}</TableHead>
              <TableHead>{t.columns.used}</TableHead>
              <TableHead>
                <span className="sr-only">{m.settings.actions}</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map((key) => (
              <TableRow key={key.id}>
                <TableCell>
                  <span className="flex items-center gap-2">
                    <span className="font-mono text-code">{keyLabel(key)}</span>
                    {key.revoked_at ? <Badge variant="secondary">{t.revoked}</Badge> : null}
                  </span>
                </TableCell>
                <TableCell label={t.columns.label}>{key.label}</TableCell>
                <TableCell label={t.columns.scopes} className="font-mono">
                  {key.scopes.join(", ")}
                </TableCell>
                <TableCell label={t.columns.app}>
                  {key.app_id ? nameOf(key.app_id) : t.allAppsShort}
                </TableCell>
                <TableCell label={t.columns.created}>
                  {f.formatDate(key.created_at, timezone)}
                </TableCell>
                <TableCell label={t.columns.used}>
                  {key.last_used_at ? f.formatDateTime(key.last_used_at, timezone) : "—"}
                </TableCell>
                <TableCell className="text-right">
                  {key.revoked_at ? null : (
                    <Button
                      variant="outline"
                      size="sm"
                      className="max-md:h-11 max-md:w-full"
                      aria-label={fmt(t.revokeItem, { key: keyLabel(key) })}
                      onClick={() => setRevoking(key)}
                    >
                      {t.revoke}
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
