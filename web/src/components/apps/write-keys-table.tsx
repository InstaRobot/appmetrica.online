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
import { PLATFORM_LABELS } from "@/components/common/platform-icon";
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
import { copyText } from "@/lib/clipboard";
import { toUiError } from "@/lib/errors";
import { formatDate, formatDateTime } from "@/lib/format";
import { useZodForm } from "@/lib/forms/use-zod-form";
import { toast } from "@/lib/toast";

// pk_7Hq2…Ab0: начало и конец ключа для таблицы и диалога отзыва (14-ux-ui-spec.md S-11).
export function shortKey(key: string): string {
  return `${key.slice(0, 7)}…${key.slice(-3)}`;
}

// F-3: метка необязательна, до 64 символов.
const schema = () => import("@/lib/forms/schemas").then((m) => m.writeKeySchema);

// Значение Select для ключа без платформы: пустую строку Radix Select не принимает.
const ANY_PLATFORM = "any";
const ANY_PLATFORM_LABEL = "Любая платформа";

// Опции платформы ключа (04-ux-diff.md §5.2): «Любая платформа», платформы приложения, остальные.
function platformOptions(appPlatforms: Platform[]): Platform[] {
  return [...appPlatforms, ...Object.values(Platform).filter((p) => !appPlatforms.includes(p))];
}

function CreateWriteKeyForm({ appId, platforms }: { appId: string; platforms: Platform[] }) {
  const queryClient = useQueryClient();
  const [created, setCreated] = useState<WriteKey | null>(null);
  const form = useZodForm(schema, {
    defaultValues: { label: "", platform: undefined },
    messages: { label: "Не длиннее 64 символов" },
    onSubmit: async ({ label, platform }) => {
      try {
        const res = await createWriteKey(appId, platform ? { label, platform } : { label });
        if (res.status !== 201) return;
        setCreated(res.data);
        void queryClient.invalidateQueries({ queryKey: getListWriteKeysQueryKey(appId) });
      } catch (error) {
        if (!form.applyServerErrors(error)) toast.error(toUiError(error).message);
      }
    },
  });

  if (created) {
    return (
      <>
        <div className="flex flex-col gap-3">
          <CopyField value={created.key} label="write-ключ" />
          <p className="text-body-sm text-fg-muted">
            Ключ публичный: его можно встраивать в приложение. Старые ключи продолжают работать,
            пока вы их не отзовёте.
          </p>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button>Готово</Button>
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
        <Label htmlFor={label.id}>Метка</Label>
        <Input
          {...label}
          autoComplete="off"
          placeholder="Например, iOS 2.x"
          value={form.values.label}
          onChange={(e) => form.setValue("label", e.target.value)}
          onBlur={() => form.blur("label")}
        />
        <FieldError id={form.errorId("label")} message={form.errors.label} />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={platform.id}>Платформа</Label>
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
              <SelectItem value={ANY_PLATFORM}>{ANY_PLATFORM_LABEL}</SelectItem>
              {platformOptions(platforms).map((p) => (
                <SelectItem key={p} value={p}>
                  {PLATFORM_LABELS[p]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Suspense>
        {form.errors.platform ? (
          <FieldError id={form.errorId("platform")} message={form.errors.platform} />
        ) : (
          <p id="write-key-platform-hint" className="text-caption text-fg-muted">
            Подставляется в события без context.platform.
          </p>
        )}
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

type RevokeDialogProps = {
  appId: string;
  writeKey: WriteKey | null;
  onClose: () => void;
};

function RevokeWriteKeyDialog({ appId, writeKey, onClose }: RevokeDialogProps) {
  const queryClient = useQueryClient();
  const revoke = useMutation({
    mutationFn: (keyId: string) => revokeWriteKey(appId, keyId),
    onSuccess: () => {
      toast.success("Ключ отозван");
      onClose();
      void queryClient.invalidateQueries({ queryKey: getListWriteKeysQueryKey(appId) });
    },
    onError: (error) => toast.error(toUiError(error).message),
  });

  return (
    <AlertDialog open={writeKey !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            Отозвать ключ {writeKey ? shortKey(writeKey.key) : ""}?
          </AlertDialogTitle>
          <AlertDialogDescription>
            Через 15 секунд все запросы с этим ключом начнут получать 401 invalid_app_key.
            Установленные версии приложения с этим ключом перестанут отправлять данные.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Отмена</AlertDialogCancel>
          <Button
            variant="destructive"
            disabled={revoke.isPending}
            onClick={() => writeKey && revoke.mutate(writeKey.id)}
          >
            {revoke.isPending ? "Отзываем…" : "Отозвать"}
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
  if (keys.length === 0) {
    return (
      <TableRow>
        <TableCell colSpan={6} className="py-6 text-center text-fg-muted">
          Ключей нет — создайте первый.
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
              <Badge variant="secondary">Отозван</Badge>
            ) : (
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={`Копировать ключ ${short}`}
                onClick={() => void copyText(key.key)}
              >
                <Copy aria-hidden="true" />
              </Button>
            )}
          </span>
        </TableCell>
        <TableCell>{key.label || "—"}</TableCell>
        <TableCell>{key.platform ? PLATFORM_LABELS[key.platform] : ANY_PLATFORM_LABEL}</TableCell>
        <TableCell>{formatDate(key.created_at, timezone)}</TableCell>
        <TableCell>{key.last_used_at ? formatDateTime(key.last_used_at, timezone) : "—"}</TableCell>
        <TableCell className="text-right">
          {key.revoked_at ? null : (
            <Button
              variant="outline"
              size="sm"
              aria-label={`Отозвать ключ ${short}`}
              onClick={() => onRevoke(key)}
            >
              Отозвать
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
  const query = useListWriteKeys(appId);
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<WriteKey | null>(null);
  const keys = query.data?.status === 200 ? query.data.data.items : undefined;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Write-ключи</CardTitle>
        <CardAction>
          <Button variant="outline" size="sm" onClick={() => setCreating(true)}>
            <Plus aria-hidden="true" />
            Создать ключ
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
          <Table>
            <caption className="sr-only">Write-ключи</caption>
            <TableHeader>
              <TableRow>
                <TableHead>Ключ</TableHead>
                <TableHead>Метка</TableHead>
                <TableHead>Платформа</TableHead>
                <TableHead>Создан</TableHead>
                <TableHead>Использован</TableHead>
                <TableHead>
                  <span className="sr-only">Действия</span>
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
            <DialogTitle>Новый write-ключ</DialogTitle>
            <DialogDescription>Ключ для отправки событий в ingest.</DialogDescription>
          </DialogHeader>
          <CreateWriteKeyForm appId={appId} platforms={platforms} />
        </DialogContent>
      </Dialog>
      <RevokeWriteKeyDialog appId={appId} writeKey={revoking} onClose={() => setRevoking(null)} />
    </Card>
  );
}
