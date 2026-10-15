"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CircleAlert } from "lucide-react";
import { useState } from "react";
import type * as zod from "zod";
import { FieldError } from "@/components/common/form-field";
import { TimezoneSelect } from "@/components/common/timezone-select";
import { ErrorState } from "@/components/feedback/error-state";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import type { ApiError } from "@/lib/api/fetcher";
import {
  getGetSettingsQueryKey,
  sendTelegramTest,
  updateSettings,
  useGetSettings,
} from "@/lib/api/gen/endpoints/admin-settings/admin-settings";
import type { Settings, UpdateSettingsBody } from "@/lib/api/gen/model";
import { useZodForm } from "@/lib/forms/use-zod-form";
import { useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";
import { TabHeader } from "./tab-header";

// F-5 (lib/forms/schemas.ts): chat_id — пусто или число, время дайджеста с шагом 15 минут.
const schema = () => import("@/lib/forms/schemas").then((m) => m.telegramSchema);

type FormData = zod.output<Awaited<ReturnType<typeof schema>>>;

// changes — тело PATCH: только поля, отличающиеся от сохранённых. Без chat_id дайджест выключен
// (ограничение settings_digest_needs_chat в Postgres).
function changes(saved: Settings, data: FormData): UpdateSettingsBody {
  const chat = data.telegram_chat_id === "" ? null : data.telegram_chat_id;
  const next: Required<UpdateSettingsBody> = {
    telegram_chat_id: chat,
    owner_timezone: data.owner_timezone,
    digest_enabled: chat !== null && data.digest_enabled,
    digest_time: data.digest_time,
    alerts_enabled: data.alerts_enabled,
  };
  const body: UpdateSettingsBody = {};
  for (const key of Object.keys(next) as (keyof UpdateSettingsBody)[]) {
    if (next[key] !== saved[key]) Object.assign(body, { [key]: next[key] });
  }
  return body;
}

function TelegramForm({ settings }: { settings: Settings }) {
  const m = useMessages();
  const t = m.settings.telegram;
  const uiError = useUiError();
  const queryClient = useQueryClient();
  const [testError, setTestError] = useState<string | null>(null);
  const form = useZodForm(schema, {
    defaultValues: {
      telegram_chat_id: settings.telegram_chat_id ?? "",
      owner_timezone: settings.owner_timezone,
      digest_enabled: settings.digest_enabled,
      digest_time: settings.digest_time,
      alerts_enabled: settings.alerts_enabled,
    },
    messages: {
      telegram_chat_id: t.errors.chatId,
      owner_timezone: t.errors.timezone,
      digest_time: t.errors.digestTime,
    },
    onSubmit: async (data) => {
      try {
        const res = await updateSettings(changes(settings, data));
        if (res.status !== 200) return;
        toast.success(t.saved);
        queryClient.setQueryData(getGetSettingsQueryKey(), res);
      } catch (error) {
        if (!form.applyServerErrors(error)) toast.error(uiError(error).message);
      }
    },
  });

  const test = useMutation({
    mutationFn: () => sendTelegramTest(),
    onMutate: () => setTestError(null),
    onSuccess: (res) => {
      if (res.status !== 200) return;
      if (res.data.sent) {
        toast.success(t.testSent);
        return;
      }
      const reason = res.data.error ? `: ${res.data.error}` : "";
      setTestError(fmt(t.testRejected, { reason }));
    },
    onError: (error) => toast.error(uiError(error).message),
  });

  const chat = form.field("telegram_chat_id");
  const timezone = form.field("owner_timezone");
  const time = form.field("digest_time");
  const hasChat = form.values.telegram_chat_id.trim() !== "";
  // Схема отличается от значений формы только trim chat_id; невалидное значение не равно сохранённому.
  const normalized = { ...form.values, telegram_chat_id: form.values.telegram_chat_id.trim() };
  const dirty = Object.keys(changes(settings, normalized)).length > 0;

  return (
    <form noValidate onSubmit={form.handleSubmit} className="flex max-w-[720px] flex-col gap-5">
      <div className="grid gap-5 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={chat.id}>{t.chatId}</Label>
          <Input
            {...chat}
            inputMode="numeric"
            autoComplete="off"
            placeholder="123456789"
            aria-describedby={chat["aria-describedby"] ?? "chat-hint"}
            value={form.values.telegram_chat_id}
            onChange={(e) => form.setValue("telegram_chat_id", e.target.value)}
            onBlur={() => form.blur("telegram_chat_id")}
          />
          {form.errors.telegram_chat_id ? (
            <FieldError
              id={form.errorId("telegram_chat_id")}
              message={form.errors.telegram_chat_id}
            />
          ) : (
            <p id="chat-hint" className="text-caption text-fg-muted">
              {t.chatHint}
            </p>
          )}
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={timezone.id}>{t.timezone}</Label>
          <TimezoneSelect
            id={timezone.id}
            ref={timezone.ref}
            invalid={timezone["aria-invalid"]}
            describedBy={timezone["aria-describedby"]}
            value={form.values.owner_timezone}
            onChange={(tz) => form.setValue("owner_timezone", tz)}
            onClose={() => form.blur("owner_timezone")}
          />
          <FieldError id={form.errorId("owner_timezone")} message={form.errors.owner_timezone} />
        </div>
      </div>

      <div className="flex flex-wrap items-start gap-x-8 gap-y-4">
        <div className="flex items-center gap-2 pt-2">
          <Switch
            id="digest_enabled"
            checked={hasChat && form.values.digest_enabled}
            disabled={!hasChat}
            onCheckedChange={(checked) => form.setValue("digest_enabled", checked)}
          />
          <Label htmlFor="digest_enabled">{t.digest}</Label>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={time.id}>{t.digestTime}</Label>
          <Input
            {...time}
            type="time"
            step={900}
            className="w-32"
            value={form.values.digest_time}
            onChange={(e) => form.setValue("digest_time", e.target.value)}
            onBlur={() => form.blur("digest_time")}
          />
          <FieldError id={form.errorId("digest_time")} message={form.errors.digest_time} />
        </div>
        <div className="flex items-center gap-2 pt-2">
          <Switch
            id="alerts_enabled"
            checked={form.values.alerts_enabled}
            onCheckedChange={(checked) => form.setValue("alerts_enabled", checked)}
          />
          <Label htmlFor="alerts_enabled">{t.alerts}</Label>
        </div>
      </div>

      {testError ? (
        <Alert variant="destructive" role="alert">
          <CircleAlert aria-hidden="true" />
          <AlertDescription>{testError}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={!settings.telegram_chat_id || test.isPending}
          onClick={() => test.mutate()}
        >
          {test.isPending ? t.sending : t.sendTest}
        </Button>
        <Button type="submit" disabled={!dirty || form.submitting}>
          {form.submitting ? t.saving : m.common.save}
        </Button>
      </div>
    </form>
  );
}

// Вкладка «Telegram» S-12 (FR-025): chat_id, часовой пояс владельца, дайджест и алерты. Тестовое сообщение
// уходит в сохранённый chat_id; отказ Telegram показывается в форме, а не toast.
export function TelegramTab() {
  const m = useMessages();
  const query = useGetSettings();
  const settings = query.data?.status === 200 ? query.data.data : undefined;

  return (
    <section aria-labelledby="telegram-title" className="flex flex-col gap-4">
      <TabHeader id="telegram-title" title={m.settings.telegram.title} />
      {query.isError && !settings ? (
        <ErrorState
          error={query.error as unknown as ApiError}
          onRetry={() => void query.refetch()}
        />
      ) : !settings ? (
        <Skeleton role="status" aria-label={m.common.loading} className="h-48 max-w-[720px]" />
      ) : (
        <TelegramForm settings={settings} />
      )}
    </section>
  );
}
