"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ChevronsUpDown, CircleAlert, Info, TriangleAlert } from "lucide-react";
import { Suspense, useId, useMemo, useRef, useState } from "react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSkeleton,
} from "@/components/common/lazy-command";
import { PlatformIcons } from "@/components/common/platform-icons";
import { Alert, AlertAction, AlertDescription } from "@/components/ui/alert";
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
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import {
  createAppMerge,
  getListAppMergesQueryKey,
  useGetMergePreview,
} from "@/lib/api/gen/endpoints/admin-apps/admin-apps";
import { getListAppsQueryKey, useListApps } from "@/lib/api/gen/endpoints/apps/apps";
import { getGetPortfolioQueryKey } from "@/lib/api/gen/endpoints/portfolio/portfolio";
import type { App, MergePreview } from "@/lib/api/gen/model";
import type { UiError } from "@/lib/errors";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";
import type { Messages } from "@/messages";

export interface MergeAppsDialogProps {
  /** Цель слияния — текущее приложение. */
  target: App;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Диалог слияния: источник — любое другое приложение, включая архивные; предпросмотр
// объёма переноса; подтверждение точным названием источника. Форма живёт внутри DialogContent, поэтому
// после закрытия диалога её состояние сбрасывается.
export function MergeAppsDialog({ target, open, onOpenChange }: MergeAppsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <MergeForm target={target} onDone={() => onOpenChange(false)} />
    </Dialog>
  );
}

type MergeTexts = Messages["apps"]["merge"];

function MergeForm({ target, onDone }: { target: App; onDone: () => void }) {
  const m = useMessages();
  const t = m.apps.merge;
  const uiError = useUiError();
  const queryClient = useQueryClient();
  const [source, setSource] = useState<App | null>(null);
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<{ field: boolean; message: string } | null>(null);
  const sourceRef = useRef<HTMLButtonElement>(null);
  const labelId = useId();
  const confirmId = useId();
  const confirmErrorId = useId();

  const preview = useGetMergePreview(
    target.id,
    { source_app_id: source?.id ?? "" },
    { query: { enabled: source !== null } },
  );
  const previewData = preview.data?.status === 200 ? preview.data.data : undefined;

  const create = useMutation({
    mutationFn: (body: { source_app_id: string; confirm_name: string }) =>
      createAppMerge(target.id, body),
  });

  const matches = source !== null && confirm.trim() === source.name;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!source || !matches || !previewData || create.isPending) return;
    setError(null);
    try {
      const res = await create.mutateAsync({
        source_app_id: source.id,
        confirm_name: confirm.trim(),
      });
      if (res.status !== 202) return;
      toast.success(t.started);
      void queryClient.invalidateQueries({ queryKey: getListAppMergesQueryKey(target.id) });
      void queryClient.invalidateQueries({ queryKey: getListAppsQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getGetPortfolioQueryKey() });
      onDone();
    } catch (err) {
      const ui = uiError(err);
      if (ui.code === "confirm_name_mismatch") setError({ field: true, message: ui.message });
      else if (ui.code === "deletion_pending") {
        setError({ field: false, message: t.deletionPending });
      } else setError({ field: false, message: ui.message });
    }
  };

  const fieldError = error?.field ? error.message : null;

  return (
    <DialogContent
      showCloseButton={false}
      className="sm:max-w-lg"
      onOpenAutoFocus={(e) => {
        e.preventDefault();
        sourceRef.current?.focus();
      }}
    >
      <form noValidate onSubmit={submit} className="grid gap-4">
        <DialogHeader>
          <DialogTitle>{t.title}</DialogTitle>
          <DialogDescription>{fmt(t.description, { name: target.name })}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-1.5">
          <Label id={labelId}>{t.source}</Label>
          <SourceCombobox
            ref={sourceRef}
            labelId={labelId}
            targetId={target.id}
            value={source}
            onChange={(app) => {
              setSource(app);
              setError(null);
            }}
          />
        </div>

        {source ? (
          <section aria-label={t.preview} aria-live="polite" className="flex flex-col gap-3">
            {previewData ? (
              <PreviewCard preview={previewData} texts={t} />
            ) : preview.isError ? (
              <Alert variant="destructive">
                <CircleAlert aria-hidden="true" />
                <AlertDescription>{previewErrorText(preview.error, uiError, t)}</AlertDescription>
                <AlertAction>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => void preview.refetch()}
                  >
                    {m.common.retry}
                  </Button>
                </AlertAction>
              </Alert>
            ) : (
              <div role="status" aria-label={m.common.loading} className="flex flex-col gap-2">
                <Skeleton className="h-5 w-full" />
                <Skeleton className="h-5 w-full" />
                <Skeleton className="h-5 w-full" />
              </div>
            )}
          </section>
        ) : null}

        <div className="flex flex-col gap-1.5">
          <Label htmlFor={confirmId}>{t.confirm}</Label>
          <Input
            id={confirmId}
            autoComplete="off"
            spellCheck={false}
            value={confirm}
            onChange={(e) => {
              setConfirm(e.target.value);
              if (error?.field) setError(null);
            }}
            aria-invalid={fieldError ? true : undefined}
            aria-describedby={fieldError ? confirmErrorId : undefined}
          />
          {fieldError ? (
            <p id={confirmErrorId} className="text-body-sm text-destructive">
              {fieldError}
            </p>
          ) : null}
        </div>

        {error && !error.field ? (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : null}

        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="ghost">
              {m.common.cancel}
            </Button>
          </DialogClose>
          <Button
            type="submit"
            variant="destructive"
            disabled={!matches || !previewData || create.isPending}
          >
            {create.isPending ? t.submitting : t.submit}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}

function previewErrorText(
  error: unknown,
  uiError: (error: unknown) => UiError,
  t: MergeTexts,
): string {
  const ui = uiError(error);
  return ui.code === "storage_unavailable" ? t.storageUnavailable : ui.message;
}

function PreviewCard({ preview, texts: t }: { preview: MergePreview; texts: MergeTexts }) {
  const f = useFormat();
  const days =
    preview.first_day && preview.last_day
      ? `${f.formatDate(preview.first_day)} — ${f.formatDate(preview.last_day)}`
      : t.noEvents;
  return (
    <>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 rounded-lg border p-3 text-body-md">
        <dt className="text-fg-subtle">{t.events}</dt>
        <dd className="tabular-nums">{f.formatInt(preview.events)}</dd>
        <dt className="text-fg-subtle">{t.days}</dt>
        <dd>{days}</dd>
        <dt className="text-fg-subtle">{t.writeKeys}</dt>
        <dd className="tabular-nums">{preview.write_keys}</dd>
      </dl>
      {preview.timezone_differs ? (
        <Alert role="note" className="border-warning/40 text-warning">
          <TriangleAlert aria-hidden="true" />
          <AlertDescription className="text-warning">
            {fmt(t.timezoneDiffers, {
              target: preview.target_timezone,
              source: preview.source_timezone,
            })}
          </AlertDescription>
        </Alert>
      ) : null}
      <Alert role="note">
        <Info aria-hidden="true" />
        <AlertDescription>{t.keysKeepWorking}</AlertDescription>
      </Alert>
    </>
  );
}

type SourceComboboxProps = {
  ref: React.Ref<HTMLButtonElement>;
  labelId: string;
  targetId: string;
  value: App | null;
  onChange: (app: App) => void;
};

// Выбор источника: активные и архивные приложения кроме цели, поиск по имени (как AppSwitcher).
function SourceCombobox({ ref, labelId, targetId, value, onChange }: SourceComboboxProps) {
  const t = useMessages().apps.merge;
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const apps = useListApps({ include_archived: true });
  const candidates = useMemo(() => {
    const items = apps.data?.status === 200 ? apps.data.data.items : [];
    return items
      .filter((app) => app.id !== targetId)
      .toSorted((a, b) => a.name.localeCompare(b.name, "ru"));
  }, [apps.data, targetId]);
  const needle = search.trim().toLocaleLowerCase("ru");
  const visible = needle
    ? candidates.filter((app) => app.name.toLocaleLowerCase("ru").includes(needle))
    : candidates;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          ref={ref}
          type="button"
          variant="outline"
          role="combobox"
          aria-labelledby={labelId}
          aria-expanded={open}
          className="w-full justify-between gap-2 font-normal"
        >
          <span className="flex min-w-0 items-center gap-2">
            {value ? <PlatformIcons platforms={value.platforms} /> : null}
            <span className="truncate">{value ? value.name : t.choose}</span>
          </span>
          <ChevronsUpDown aria-hidden="true" className="text-fg-subtle" />
        </Button>
      </PopoverTrigger>
      {/* z-modal: z-dropdown ниже диалога, и список оказался бы под ним; портал popover идёт после диалога. */}
      <PopoverContent align="start" className="z-modal w-(--radix-popover-trigger-width) p-0">
        <Suspense fallback={<CommandSkeleton />}>
          <Command shouldFilter={false}>
            <CommandInput placeholder={t.search} value={search} onValueChange={setSearch} />
            <CommandList>
              <CommandEmpty>{needle ? t.notFound : t.noOther}</CommandEmpty>
              {visible.length > 0 ? (
                <CommandGroup>
                  {visible.map((app) => (
                    <CommandItem
                      key={app.id}
                      value={app.id}
                      onSelect={() => {
                        onChange(app);
                        setOpen(false);
                        setSearch("");
                      }}
                    >
                      <PlatformIcons platforms={app.platforms} />
                      <span className="truncate">{app.name}</span>
                      {app.status === "archived" ? (
                        <span className="ml-auto text-label-sm text-fg-subtle">{t.archived}</span>
                      ) : null}
                    </CommandItem>
                  ))}
                </CommandGroup>
              ) : null}
            </CommandList>
          </Command>
        </Suspense>
      </PopoverContent>
    </Popover>
  );
}
