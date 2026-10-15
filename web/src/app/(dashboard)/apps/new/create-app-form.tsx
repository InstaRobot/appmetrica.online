"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { Route } from "next";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FieldError, Required } from "@/components/common/form-field";
import { PlatformToggleGroup } from "@/components/common/platform-toggle-group";
import { TimezoneSelect } from "@/components/common/timezone-select";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { createApp } from "@/lib/api/gen/endpoints/admin-apps/admin-apps";
import { useGetSettings } from "@/lib/api/gen/endpoints/admin-settings/admin-settings";
import { getListAppsQueryKey } from "@/lib/api/gen/endpoints/apps/apps";
import { getGetPortfolioQueryKey } from "@/lib/api/gen/endpoints/portfolio/portfolio";
import type { CreateAppBody as CreateAppRequest } from "@/lib/api/gen/model";
import { useZodForm } from "@/lib/forms/use-zod-form";
import { useMessages } from "@/lib/i18n/client";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";

// F-1 (14-ux-ui-spec.md §7.1): правила — из контракта, плюс trim названия и проверка IANA-зоны.
const schema = () => import("@/lib/forms/schemas").then((m) => m.createAppSchema);

// S-04: создание приложения (FR-009) с одной или несколькими платформами.
// Сервер создаёт ключ записи на каждую платформу; после 201 — экран интеграции со сниппетами.
export function CreateAppForm({ defaultTimezone }: { defaultTimezone: string }) {
  const m = useMessages();
  const t = m.apps;
  const uiError = useUiError();
  const router = useRouter();
  const queryClient = useQueryClient();
  const create = useMutation({ mutationFn: (body: CreateAppRequest) => createApp(body) });

  const form = useZodForm(schema, {
    defaultValues: { name: "", platforms: [], timezone: defaultTimezone },
    messages: {
      name: { too_small: t.fields.nameRequired, too_big: t.fields.tooLong },
      platforms: t.fields.platformsRequired,
      timezone: t.fields.timezoneRequired,
    },
    serverErrors: { app_name_taken: "name" },
    onSubmit: async (body) => {
      try {
        const res = await create.mutateAsync(body);
        if (res.status !== 201) return;
        toast.success(t.create.created);
        void queryClient.invalidateQueries({ queryKey: getListAppsQueryKey() });
        void queryClient.invalidateQueries({ queryKey: getGetPortfolioQueryKey() });
        router.push(`/apps/${res.data.app.id}/integration` as Route);
      } catch (error) {
        if (!form.applyServerErrors(error)) toast.error(uiError(error).message);
      }
    },
  });

  const name = form.field("name");
  const platforms = form.field("platforms");
  const timezone = form.field("timezone");

  return (
    <form noValidate onSubmit={form.handleSubmit} className="max-w-[560px]">
      <Card>
        <CardContent className="flex flex-col gap-5">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={name.id}>
              {t.fields.name} <Required />
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
            <Label id="platforms-label">
              {t.fields.platforms} <Required />
            </Label>
            <PlatformToggleGroup
              ref={platforms.ref}
              aria-labelledby="platforms-label"
              aria-describedby={platforms["aria-describedby"] ?? "platforms-hint"}
              disabled={form.submitting}
              value={form.values.platforms}
              onValueChange={(value) => form.setValue("platforms", value)}
            />
            {form.errors.platforms ? (
              <FieldError id={form.errorId("platforms")} message={form.errors.platforms} />
            ) : (
              <p id="platforms-hint" className="text-caption text-fg-muted">
                {t.create.platformsHint}
              </p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor={timezone.id}>
              {t.fields.timezone} <Required />
            </Label>
            <TimezoneSelect
              id={timezone.id}
              ref={timezone.ref}
              required
              invalid={timezone["aria-invalid"]}
              describedBy={timezone["aria-describedby"] ?? "timezone-hint"}
              value={form.values.timezone}
              onChange={(tz) => form.setValue("timezone", tz)}
              onClose={() => form.blur("timezone")}
            />
            {form.errors.timezone ? (
              <FieldError id={form.errorId("timezone")} message={form.errors.timezone} />
            ) : (
              <p id="timezone-hint" className="text-caption text-fg-muted">
                {t.create.timezoneHint}
              </p>
            )}
          </div>

          {/* На телефоне кнопки во всю ширину, основное действие — сверху. */}
          <div className="flex justify-end gap-2 max-md:flex-col-reverse">
            <Button variant="outline" asChild>
              <Link href="/portfolio">{m.common.cancel}</Link>
            </Button>
            <Button type="submit" disabled={form.submitting}>
              {form.submitting ? t.actions.creating : t.actions.create}
            </Button>
          </div>
        </CardContent>
      </Card>
    </form>
  );
}

// Заголовок S-04 на языке интерфейса: словарь есть только у клиентских компонентов под MessagesProvider.
export function NewAppHeader() {
  return <PageHeader title={useMessages().apps.create.title} />;
}

// Часовой пояс по умолчанию — settings.owner_timezone; без настроек — зона браузера.
export function NewAppForm() {
  const loading = useMessages().common.loading;
  const settings = useGetSettings();
  if (settings.isPending) {
    return <Skeleton role="status" aria-label={loading} className="h-80 max-w-[560px]" />;
  }
  const defaultTimezone =
    settings.data?.status === 200
      ? settings.data.data.owner_timezone
      : Intl.DateTimeFormat().resolvedOptions().timeZone;
  return <CreateAppForm defaultTimezone={defaultTimezone} />;
}
