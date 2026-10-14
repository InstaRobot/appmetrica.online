"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { lazy, Suspense, useState, useSyncExternalStore } from "react";
import { FieldError, Required } from "@/components/common/form-field";
import { TimezoneSelect } from "@/components/common/timezone-select";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { updateApp } from "@/lib/api/gen/endpoints/admin-apps/admin-apps";
import { getGetAppQueryKey, getListAppsQueryKey } from "@/lib/api/gen/endpoints/apps/apps";
import { getGetPortfolioQueryKey } from "@/lib/api/gen/endpoints/portfolio/portfolio";
import type { App, Platform, UpdateAppBody } from "@/lib/api/gen/model";
import { isDemoInBrowser } from "@/lib/demo-mode";
import { toUiError } from "@/lib/errors";
import { useZodForm } from "@/lib/forms/use-zod-form";
import { toast } from "@/lib/toast";

// Job close_day закрывает день D−8 (02-tech-spec.md FR-022): у приложения старше 8 суток есть закрытые дни.
const CLOSED_DAY_LAG_MS = 8 * 24 * 60 * 60 * 1000;

export function hasClosedDays(createdAt: string, now: Date = new Date()): boolean {
  const created = Date.parse(createdAt);
  return !Number.isNaN(created) && now.getTime() - created >= CLOSED_DAY_LAG_MS;
}

// F-2 (14-ux-ui-spec.md §7.1): как F-1 — trim названия 1–64 и IANA-зона.
const schema = () => import("@/lib/forms/schemas").then((m) => m.appSettingsSchema);

const messages = {
  name: { too_small: "Введите название", too_big: "Не длиннее 64 символов" },
  timezone: "Выберите часовой пояс",
};

// Карточки S-11: «Основное» (FR-009) и «Платформы» (04-ux-diff.md §5.2). У каждой своя кнопка
// «Сохранить» и свой PATCH.
export function AppSettingsForm({ app }: { app: App }) {
  return (
    <>
      <AppBasicsCard app={app} />
      <AppPlatformsCard app={app} />
    </>
  );
}

function useSavedApp(app: App) {
  const queryClient = useQueryClient();
  const update = useMutation({
    mutationFn: (body: UpdateAppBody) => updateApp(app.id, body),
  });
  const save = async (body: UpdateAppBody): Promise<boolean> => {
    const res = await update.mutateAsync(body);
    if (res.status !== 200) return false;
    queryClient.setQueryData(getGetAppQueryKey(app.id), res);
    void queryClient.invalidateQueries({ queryKey: getListAppsQueryKey() });
    void queryClient.invalidateQueries({ queryKey: getGetPortfolioQueryKey() });
    return true;
  };
  return { save, pending: update.isPending };
}

// «Основное»: название и часовой пояс. Отправляются только изменённые поля; смена пояса предупреждает,
// что уже закрытые дни остаются в прежнем поясе.
function AppBasicsCard({ app }: { app: App }) {
  const { save } = useSavedApp(app);

  const form = useZodForm(schema, {
    defaultValues: { name: app.name, timezone: app.timezone },
    messages,
    serverErrors: { app_name_taken: "name" },
    onSubmit: async ({ name, timezone }) => {
      const body: UpdateAppBody = {
        ...(name !== app.name ? { name } : {}),
        ...(timezone !== app.timezone ? { timezone } : {}),
      };
      try {
        if (await save(body)) toast.success("Настройки сохранены");
      } catch (error) {
        if (!form.applyServerErrors(error)) toast.error(toUiError(error).message);
      }
    },
  });

  const name = form.field("name");
  const timezone = form.field("timezone");
  const dirty = form.values.name.trim() !== app.name || form.values.timezone !== app.timezone;
  const warnClosedDays = form.values.timezone !== app.timezone && hasClosedDays(app.created_at);
  const timezoneDescribedBy = [
    timezone["aria-describedby"] ?? "timezone-hint",
    warnClosedDays ? "timezone-warning" : undefined,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <Card>
      <CardHeader>
        <CardTitle id="app-basics-title">Основное</CardTitle>
      </CardHeader>
      <CardContent>
        <form
          noValidate
          aria-labelledby="app-basics-title"
          onSubmit={form.handleSubmit}
          className="flex flex-col gap-5"
        >
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={name.id}>
              Название <Required />
            </Label>
            <Input
              {...name}
              autoComplete="off"
              aria-required="true"
              value={form.values.name}
              onChange={(e) => form.setValue("name", e.target.value)}
              onBlur={() => form.blur("name")}
            />
            <FieldError id={form.errorId("name")} message={form.errors.name} />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor={timezone.id}>
              Часовой пояс <Required />
            </Label>
            <TimezoneSelect
              id={timezone.id}
              ref={timezone.ref}
              required
              invalid={timezone["aria-invalid"]}
              describedBy={timezoneDescribedBy}
              value={form.values.timezone}
              onChange={(tz) => form.setValue("timezone", tz)}
              onClose={() => form.blur("timezone")}
            />
            {form.errors.timezone ? (
              <FieldError id={form.errorId("timezone")} message={form.errors.timezone} />
            ) : (
              <p id="timezone-hint" className="text-caption text-fg-muted">
                Меняет границы суток в отчётах; закрытые дни не пересчитываются.
              </p>
            )}
            {warnClosedDays ? (
              <Alert
                id="timezone-warning"
                role="status"
                aria-label="Смена часового пояса"
                className="border-warning/40 text-warning"
              >
                <TriangleAlert aria-hidden="true" />
                <AlertDescription className="text-warning">
                  Закрытые дни не пересчитываются: дни старше 8 суток уже посчитаны в поясе{" "}
                  {app.timezone}. Новый пояс применится к остальным дням.
                </AlertDescription>
              </Alert>
            ) : null}
          </div>

          <div className="flex justify-end">
            <Button type="submit" disabled={!dirty || form.submitting}>
              {form.submitting ? "Сохраняем…" : "Сохранить"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

// Radix ToggleGroup не входит в initial JS страницы настроек (бюджет NFR-006): чипы догружаются, пока
// на их месте рамка того же вида.
const PlatformToggleGroup = lazy(() =>
  import("@/components/common/platform-toggle-group").then((m) => ({
    default: m.PlatformToggleGroup,
  })),
);

const noSubscribe = () => () => {};

// Демо-режим известен только в браузере (cookie am_demo): на сервере рендер без него.
function useDemo(): boolean {
  return useSyncExternalStore(noSubscribe, isDemoInBrowser, () => false);
}

function samePlatforms(a: Platform[], b: Platform[]): boolean {
  return a.length === b.length && a.every((p) => b.includes(p));
}

// «Платформы» (#platforms): набор платформ приложения, PATCH { platforms }. Пустой набор не сохраняется;
// в демо мутации недоступны, поэтому чипы и кнопка выключены.
function AppPlatformsCard({ app }: { app: App }) {
  const { save, pending } = useSavedApp(app);
  const demo = useDemo();
  const [platforms, setPlatforms] = useState<Platform[]>(app.platforms);
  const empty = platforms.length === 0;
  const dirty = !samePlatforms(platforms, app.platforms);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    try {
      if (await save({ platforms })) toast.success("Платформы сохранены");
    } catch (error) {
      toast.error(toUiError(error).message);
    }
  };

  return (
    <section id="platforms" aria-labelledby="app-platforms-title" className="scroll-mt-6">
      <Card>
        <CardHeader>
          <CardTitle id="app-platforms-title">Платформы</CardTitle>
        </CardHeader>
        <CardContent>
          <form noValidate onSubmit={submit} className="flex flex-col gap-3">
            <Suspense fallback={<div className="h-8 w-full rounded-lg border border-input" />}>
              <PlatformToggleGroup
                aria-labelledby="app-platforms-title"
                aria-describedby={empty ? "platforms-empty platforms-hint" : "platforms-hint"}
                value={platforms}
                onValueChange={setPlatforms}
                disabled={demo}
              />
            </Suspense>
            <FieldError
              id="platforms-empty"
              message={empty ? "Оставьте хотя бы одну платформу" : undefined}
            />
            <p id="platforms-hint" className="text-caption text-fg-muted">
              Убрать платформу — не удалить её данные: события и ключи сохраняются.
            </p>
            <div className="flex justify-end">
              <Button type="submit" disabled={demo || empty || !dirty || pending}>
                {pending ? "Сохраняем…" : "Сохранить"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </section>
  );
}
