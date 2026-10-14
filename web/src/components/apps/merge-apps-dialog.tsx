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
import { toUiError } from "@/lib/errors";
import { formatDate, formatInt } from "@/lib/format";
import { toast } from "@/lib/toast";

export interface MergeAppsDialogProps {
  /** Цель слияния — текущее приложение. */
  target: App;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Диалог слияния (04-ux-diff.md §5.3): источник — любое другое приложение, включая архивные; предпросмотр
// объёма переноса; подтверждение точным названием источника. Форма живёт внутри DialogContent, поэтому
// после закрытия диалога её состояние сбрасывается.
export function MergeAppsDialog({ target, open, onOpenChange }: MergeAppsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <MergeForm target={target} onDone={() => onOpenChange(false)} />
    </Dialog>
  );
}

function MergeForm({ target, onDone }: { target: App; onDone: () => void }) {
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
      toast.success("Объединение запущено");
      void queryClient.invalidateQueries({ queryKey: getListAppMergesQueryKey(target.id) });
      void queryClient.invalidateQueries({ queryKey: getListAppsQueryKey() });
      void queryClient.invalidateQueries({ queryKey: getGetPortfolioQueryKey() });
      onDone();
    } catch (err) {
      const ui = toUiError(err);
      if (ui.code === "confirm_name_mismatch") setError({ field: true, message: ui.message });
      else if (ui.code === "deletion_pending") {
        setError({
          field: false,
          message:
            "У одного из приложений есть невыполненная заявка на удаление — дождитесь её выполнения",
        });
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
          <DialogTitle>Объединить приложения</DialogTitle>
          <DialogDescription>
            Данные выбранного приложения переедут в «{target.name}».
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-1.5">
          <Label id={labelId}>Приложение-источник</Label>
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
          <section aria-label="Что переедет" aria-live="polite" className="flex flex-col gap-3">
            {previewData ? (
              <PreviewCard preview={previewData} />
            ) : preview.isError ? (
              <Alert variant="destructive">
                <CircleAlert aria-hidden="true" />
                <AlertDescription>{previewErrorText(preview.error)}</AlertDescription>
                <AlertAction>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => void preview.refetch()}
                  >
                    Повторить
                  </Button>
                </AlertAction>
              </Alert>
            ) : (
              <div role="status" aria-label="Загрузка…" className="flex flex-col gap-2">
                <Skeleton className="h-5 w-full" />
                <Skeleton className="h-5 w-full" />
                <Skeleton className="h-5 w-full" />
              </div>
            )}
          </section>
        ) : null}

        <div className="flex flex-col gap-1.5">
          <Label htmlFor={confirmId}>Введите название источника, чтобы подтвердить</Label>
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
              Отмена
            </Button>
          </DialogClose>
          <Button
            type="submit"
            variant="destructive"
            disabled={!matches || !previewData || create.isPending}
          >
            {create.isPending ? "Объединяем…" : "Объединить"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}

function previewErrorText(error: unknown): string {
  const ui = toUiError(error);
  return ui.code === "storage_unavailable" ? "Хранилище данных временно недоступно." : ui.message;
}

function PreviewCard({ preview }: { preview: MergePreview }) {
  const days =
    preview.first_day && preview.last_day
      ? `${formatDate(preview.first_day)} — ${formatDate(preview.last_day)}`
      : "нет событий";
  return (
    <>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 rounded-lg border p-3 text-body-md">
        <dt className="text-fg-subtle">Событий</dt>
        <dd className="tabular-nums">{formatInt(preview.events)}</dd>
        <dt className="text-fg-subtle">Дни данных</dt>
        <dd>{days}</dd>
        <dt className="text-fg-subtle">Ключей записи</dt>
        <dd className="tabular-nums">{preview.write_keys}</dd>
      </dl>
      {preview.timezone_differs ? (
        <Alert role="note" className="border-warning/40 text-warning">
          <TriangleAlert aria-hidden="true" />
          <AlertDescription className="text-warning">
            {`Часовой пояс останется «${preview.target_timezone}»: дни источника (${preview.source_timezone}) пересчитаются.`}
          </AlertDescription>
        </Alert>
      ) : null}
      <Alert role="note">
        <Info aria-hidden="true" />
        <AlertDescription>
          Ключи источника продолжат работать — выпущенные версии приложений обновлять не нужно.
        </AlertDescription>
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
            <span className="truncate">{value ? value.name : "Выберите приложение"}</span>
          </span>
          <ChevronsUpDown aria-hidden="true" className="text-fg-subtle" />
        </Button>
      </PopoverTrigger>
      {/* z-modal: z-dropdown ниже диалога, и список оказался бы под ним; портал popover идёт после диалога. */}
      <PopoverContent align="start" className="z-modal w-(--radix-popover-trigger-width) p-0">
        <Suspense fallback={<CommandSkeleton />}>
          <Command shouldFilter={false}>
            <CommandInput
              placeholder="Найти приложение…"
              value={search}
              onValueChange={setSearch}
            />
            <CommandList>
              <CommandEmpty>{needle ? "Ничего не найдено" : "Других приложений нет"}</CommandEmpty>
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
                        <span className="ml-auto text-label-sm text-fg-subtle">в архиве</span>
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
