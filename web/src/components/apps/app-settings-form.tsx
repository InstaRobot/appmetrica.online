"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { FieldError, Required } from "@/components/common/form-field";
import { PLATFORM_LABELS } from "@/components/common/platform-icon";
import { TimezoneSelect } from "@/components/common/timezone-select";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { updateApp } from "@/lib/api/gen/endpoints/admin-apps/admin-apps";
import { getGetAppQueryKey, getListAppsQueryKey } from "@/lib/api/gen/endpoints/apps/apps";
import { getGetPortfolioQueryKey } from "@/lib/api/gen/endpoints/portfolio/portfolio";
import type { App, UpdateAppBody } from "@/lib/api/gen/model";
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

// Карточка «Основное» S-11 (FR-009): название и часовой пояс; платформа не меняется. Отправляются только
// изменённые поля; смена пояса предупреждает, что уже закрытые дни остаются в прежнем поясе.
export function AppSettingsForm({ app }: { app: App }) {
  const queryClient = useQueryClient();
  const update = useMutation({
    mutationFn: (body: UpdateAppBody) => updateApp(app.id, body),
  });

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
        const res = await update.mutateAsync(body);
        if (res.status !== 200) return;
        toast.success("Настройки сохранены");
        queryClient.setQueryData(getGetAppQueryKey(app.id), res);
        void queryClient.invalidateQueries({ queryKey: getListAppsQueryKey() });
        void queryClient.invalidateQueries({ queryKey: getGetPortfolioQueryKey() });
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
            <span className="text-label-md">Платформа</span>
            <p className="text-body-md text-fg-muted">
              {`${PLATFORM_LABELS[app.platform]} (изменить нельзя)`}
            </p>
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
