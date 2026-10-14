"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { Route } from "next";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FieldError, Required } from "@/components/common/form-field";
import { PLATFORM_LABELS } from "@/components/common/platform-icon";
import { TimezoneSelect } from "@/components/common/timezone-select";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { createApp } from "@/lib/api/gen/endpoints/admin-apps/admin-apps";
import { useGetSettings } from "@/lib/api/gen/endpoints/admin-settings/admin-settings";
import { getListAppsQueryKey } from "@/lib/api/gen/endpoints/apps/apps";
import { getGetPortfolioQueryKey } from "@/lib/api/gen/endpoints/portfolio/portfolio";
import { type CreateAppBody as CreateAppRequest, Platform } from "@/lib/api/gen/model";
import { toUiError } from "@/lib/errors";
import { useZodForm } from "@/lib/forms/use-zod-form";
import { toast } from "@/lib/toast";

// F-1 (14-ux-ui-spec.md §7.1): правила — из контракта, плюс trim названия и проверка IANA-зоны.
const schema = () => import("@/lib/forms/schemas").then((m) => m.createAppSchema);

const messages = {
  name: { too_small: "Введите название", too_big: "Не длиннее 64 символов" },
  platform: "Выберите платформу",
  timezone: "Выберите часовой пояс",
};

// Порядок и подписи опций S-04; «server» здесь «Сервер», как в спецификации экрана.
const PLATFORM_OPTIONS = Object.values(Platform).map((value) => ({
  value,
  label: value === Platform.server ? "Сервер" : PLATFORM_LABELS[value],
}));

// S-04: создание приложения (FR-009). После 201 — экран интеграции с write-ключом и сниппетами.
export function CreateAppForm({ defaultTimezone }: { defaultTimezone: string }) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const create = useMutation({ mutationFn: (body: CreateAppRequest) => createApp(body) });

  const form = useZodForm(schema, {
    defaultValues: { name: "", platform: "", timezone: defaultTimezone },
    messages,
    serverErrors: { app_name_taken: "name" },
    onSubmit: async (body) => {
      try {
        const res = await create.mutateAsync(body);
        if (res.status !== 201) return;
        toast.success("Приложение создано");
        void queryClient.invalidateQueries({ queryKey: getListAppsQueryKey() });
        void queryClient.invalidateQueries({ queryKey: getGetPortfolioQueryKey() });
        router.push(`/apps/${res.data.app.id}/integration` as Route);
      } catch (error) {
        if (!form.applyServerErrors(error)) toast.error(toUiError(error).message);
      }
    },
  });

  const name = form.field("name");
  const platform = form.field("platform");
  const timezone = form.field("timezone");

  return (
    <form noValidate onSubmit={form.handleSubmit} className="max-w-[560px]">
      <Card>
        <CardContent className="flex flex-col gap-5">
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
            <Label htmlFor={platform.id}>
              Платформа <Required />
            </Label>
            <Select
              value={form.values.platform}
              onValueChange={(value) => form.setValue("platform", value)}
              onOpenChange={(open) => {
                if (!open) form.blur("platform");
              }}
            >
              <SelectTrigger
                {...platform}
                aria-required="true"
                aria-describedby={platform["aria-describedby"] ?? "platform-hint"}
                className="w-full"
              >
                <SelectValue placeholder="Выберите платформу" />
              </SelectTrigger>
              <SelectContent>
                {PLATFORM_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {form.errors.platform ? (
              <FieldError id={form.errorId("platform")} message={form.errors.platform} />
            ) : (
              <p id="platform-hint" className="text-caption text-fg-muted">
                Используется по умолчанию, если в событии не передан context.platform.
              </p>
            )}
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
              describedBy={timezone["aria-describedby"] ?? "timezone-hint"}
              value={form.values.timezone}
              onChange={(tz) => form.setValue("timezone", tz)}
              onClose={() => form.blur("timezone")}
            />
            {form.errors.timezone ? (
              <FieldError id={form.errorId("timezone")} message={form.errors.timezone} />
            ) : (
              <p id="timezone-hint" className="text-caption text-fg-muted">
                Границы суток в отчётах.
              </p>
            )}
          </div>

          <div className="flex justify-end gap-2">
            <Button variant="outline" asChild>
              <Link href="/portfolio">Отмена</Link>
            </Button>
            <Button type="submit" disabled={form.submitting}>
              {form.submitting ? "Создаём…" : "Создать"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </form>
  );
}

// Часовой пояс по умолчанию — settings.owner_timezone; без настроек — зона браузера.
export function NewAppForm() {
  const settings = useGetSettings();
  if (settings.isPending) {
    return <Skeleton role="status" aria-label="Загрузка…" className="h-80 max-w-[560px]" />;
  }
  const defaultTimezone =
    settings.data?.status === 200
      ? settings.data.data.owner_timezone
      : Intl.DateTimeFormat().resolvedOptions().timeZone;
  return <CreateAppForm defaultTimezone={defaultTimezone} />;
}
