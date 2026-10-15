"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Copy, Plus } from "lucide-react";
import { Suspense, useState } from "react";
import { CopyField } from "@/components/common/copy-field";
import { FieldError } from "@/components/common/form-field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/common/lazy-select";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
  createWriteKey,
  getListWriteKeysQueryKey,
  revokeWriteKey,
  useListWriteKeys,
} from "@/lib/api/gen/endpoints/admin-keys/admin-keys";
import { Platform, type WriteKey } from "@/lib/api/gen/model";
import { useCopyText } from "@/lib/clipboard";
import { useZodForm } from "@/lib/forms/use-zod-form";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";

// pk_7Hq2…Ab0: начало и конец ключа для таблицы и диалога отзыва (14-ux-ui-spec.md S-11).
export function shortKey(key: string): string {
  return `${key.slice(0, 7)}…${key.slice(-3)}`;
}

// F-3: метка необязательна, до 64 символов.
const schema = () => import("@/lib/forms/schemas").then((m) => m.writeKeySchema);

// Значение Select для ключа без платформы: пустую строку Radix Select не принимает.
const ANY_PLATFORM = "any";

// Опции платформы ключа: «Любая платформа», платформы приложения, остальные.
function platformOptions(appPlatforms: Platform[]): Platform[] {
  return [...appPlatforms, ...Object.values(Platform).filter((p) => !appPlatforms.includes(p))];
}

function CreateWriteKeyForm({ appId, platforms }: { appId: string; platforms: Platform[] }) {
  const platformLabels = useMessages().common.platforms;
  const m = useMessages();
  const t = m.apps.writeKeys;
  const uiError = useUiError();
  const queryClient = useQueryClient();
  const [created, setCreated] = useState<WriteKey | null>(null);
  const form = useZodForm(schema, {
    defaultValues: { label: "", platform: undefined },
    messages: { label: m.apps.fields.tooLong },
    onSubmit: async ({ label, platform }) => {
      try {
        const res = await createWriteKey(appId, platform ? { label, platform } : { label });
        if (res.status !== 201) return;
        setCreated(res.data);
        void queryClient.invalidateQueries({ queryKey: getListWriteKeysQueryKey(appId) });
      } catch (error) {
        if (!form.applyServerErrors(error)) toast.error(uiError(error).message);
      }
    },
  });

  if (created) {
    return (
      <>
        <div className="flex flex-col gap-3">
          <CopyField value={created.key} label={t.keyField} />
          <p className="text-body-sm text-fg-muted">{t.createdHint}</p>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button>{t.done}</Button>
          </DialogClose>
        </DialogFooter>
      </>
    );
  }

  const label = form.field("label");
  const platform = form.field("platform");
  return (
    <form noValidate onSubmit={form.handleSubmit} className="grid gap-4">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={label.id}>{t.label}</Label>
        <Input
          {...label}
          autoComplete="off"
          placeholder={t.labelPlaceholder}
          value={form.values.label}
          onChange={(e) => form.setValue("label", e.target.value)}
          onBlur={() => form.blur("label")}
        />
        <FieldError id={form.errorId("label")} message={form.errors.label} />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={platform.id}>{t.platform}</Label>
        <Suspense fallback={<div className="h-8 w-full rounded-lg border border-input" />}>
          <Select
            name="platform"
            value={form.values.platform ?? ANY_PLATFORM}
            onValueChange={(value) =>
              form.setValue("platform", value === ANY_PLATFORM ? undefined : (value as Platform))
            }
          >
            <SelectTrigger
              {...platform}
              aria-describedby={platform["aria-describedby"] ?? "write-key-platform-hint"}
              className="w-full"
            >
              <SelectValue />
            </SelectTrigger>
            {/* Список в диалоге: z-dropdown ниже затемнения z-overlay, клики уходили бы в него. */}
            <SelectContent className="z-modal">
              <SelectItem value={ANY_PLATFORM}>{t.anyPlatform}</SelectItem>
              {platformOptions(platforms).map((p) => (
                <SelectItem key={p} value={p}>
                  {platformLabels[p]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Suspense>
        {form.errors.platform ? (
          <FieldError id={form.errorId("platform")} message={form.errors.platform} />
        ) : (
          <p id="write-key-platform-hint" className="text-caption text-fg-muted">
            {t.platformHint}
          </p>
        )}
      </div>
      <DialogFooter>
        <DialogClose asChild>
          <Button type="button" variant="outline">
            {m.common.cancel}
          </Button>
        </DialogClose>
        <Button type="submit" disabled={form.submitting}>
          {form.submitting ? m.apps.actions.creating : m.apps.actions.create}
        </Button>
      </DialogFooter>
    </form>
  );
}

type RevokeDialogProps = {
  appId: string;
  writeKey: WriteKey | null;
  onClose: () => void;
};

function RevokeWriteKeyDialog({ appId, writeKey, onClose }: RevokeDialogProps) {
  const m = useMessages();
  const t = m.apps.writeKeys;
  const uiError = useUiError();
  const queryClient = useQueryClient();
  const revoke = useMutation({
    mutationFn: (keyId: string) => revokeWriteKey(appId, keyId),
    onSuccess: () => {
      toast.success(t.revoked);
      onClose();
      void queryClient.invalidateQueries({ queryKey: getListWriteKeysQueryKey(appId) });
    },
    onError: (error) => toast.error(uiError(error).message),
  });

  return (
    <AlertDialog open={writeKey !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {fmt(t.revokeTitle, { key: writeKey ? shortKey(writeKey.key) : "" })}
          </AlertDialogTitle>
          <AlertDialogDescription>{t.revokeDescription}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{m.common.cancel}</AlertDialogCancel>
          <Button
            variant="destructive"
            disabled={revoke.isPending}
            onClick={() => writeKey && revoke.mutate(writeKey.id)}
          >
            {revoke.isPending ? t.revoking : t.revoke}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function WriteKeyRows({
  keys,
  timezone,
  onRevoke,
}: {
  keys: WriteKey[];
  timezone: string;
  onRevoke: (key: WriteKey) => void;
}) {
  const copy = useCopyText();
  const platformLabels = useMessages().common.platforms;
  const t = useMessages().apps.writeKeys;
  const f = useFormat();
  if (keys.length === 0) {
    return (
      <TableRow>
        <TableCell colSpan={6} className="py-6 text-center text-fg-muted">
          {t.empty}
        </TableCell>
      </TableRow>
    );
  }
  return keys.map((key) => {
    const short = shortKey(key.key);
    return (
      <TableRow key={key.id}>
        <TableCell>
          <span className="flex items-center gap-2">
            <span className="font-mono text-code">{short}</span>
            {key.revoked_at ? (
              <Badge variant="secondary">{t.revokedBadge}</Badge>
            ) : (
              <Button
                variant="ghost"
                size="icon-xs"
                className="max-md:size-11"
                aria-label={fmt(t.copyKey, { key: short })}
                onClick={() => void copy(key.key)}
              >
                <Copy aria-hidden="true" />
              </Button>
            )}
          </span>
        </TableCell>
        <TableCell label={t.columns.label}>{key.label || "—"}</TableCell>
        <TableCell label={t.columns.platform}>
          {key.platform ? platformLabels[key.platform] : t.anyPlatform}
        </TableCell>
        <TableCell label={t.columns.created}>{f.formatDate(key.created_at, timezone)}</TableCell>
        <TableCell label={t.columns.used}>
          {key.last_used_at ? f.formatDateTime(key.last_used_at, timezone) : "—"}
        </TableCell>
        <TableCell className="text-right">
          {key.revoked_at ? null : (
            <Button
              variant="outline"
              size="sm"
              className="max-md:h-11 max-md:w-full"
              aria-label={fmt(t.revokeKey, { key: short })}
              onClick={() => onRevoke(key)}
            >
              {t.revoke}
            </Button>
          )}
        </TableCell>
      </TableRow>
    );
  });
}

type WriteKeysTableProps = { appId: string; platforms: Platform[]; timezone: string };

// Карточка «Write-ключи» S-11 (FR-010): несколько активных ключей, создание с показом ключа и отзыв.
export function WriteKeysTable({ appId, platforms, timezone }: WriteKeysTableProps) {
  const t = useMessages().apps.writeKeys;
  const query = useListWriteKeys(appId);
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<WriteKey | null>(null);
  const keys = query.data?.status === 200 ? query.data.data.items : undefined;

  // #write-keys — цель ссылки «Создать ключ для {платформа}» вкладки «Интеграция».
  return (
    <section id="write-keys" aria-labelledby="write-keys-title" className="scroll-mt-6">
      <Card>
        <CardHeader>
          <CardTitle id="write-keys-title">{t.title}</CardTitle>
          <CardAction>
            <Button variant="outline" size="sm" onClick={() => setCreating(true)}>
              <Plus aria-hidden="true" />
              {t.create}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          {query.isError && !keys ? (
            <ErrorState
              error={query.error as unknown as ApiError}
              onRetry={() => void query.refetch()}
            />
          ) : !keys ? (
            <TableSkeleton rows={2} />
          ) : (
            <Table stackOnMobile>
              <caption className="sr-only">{t.title}</caption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t.columns.key}</TableHead>
                  <TableHead>{t.columns.label}</TableHead>
                  <TableHead>{t.columns.platform}</TableHead>
                  <TableHead>{t.columns.created}</TableHead>
                  <TableHead>{t.columns.used}</TableHead>
                  <TableHead>
                    <span className="sr-only">{t.columns.actions}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <WriteKeyRows keys={keys} timezone={timezone} onRevoke={setRevoking} />
              </TableBody>
            </Table>
          )}
        </CardContent>

        <Dialog open={creating} onOpenChange={setCreating}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t.dialogTitle}</DialogTitle>
              <DialogDescription>{t.dialogDescription}</DialogDescription>
            </DialogHeader>
            <CreateWriteKeyForm appId={appId} platforms={platforms} />
          </DialogContent>
        </Dialog>
        <RevokeWriteKeyDialog appId={appId} writeKey={revoking} onClose={() => setRevoking(null)} />
      </Card>
    </section>
  );
}
