import * as zod from "zod";
import { isTimezone } from "@/components/common/timezone-select";
import { CreateAppBody } from "@/lib/api/gen/zod/admin-apps/admin-apps.zod";
import {
  CreateApiKeyBody,
  CreateWriteKeyBody,
  createWriteKeyBodyLabelMax,
} from "@/lib/api/gen/zod/admin-keys/admin-keys.zod";

// Zod-схемы форм дашборда (14-ux-ui-spec.md §7.1). Модуль подключается только динамически —
// `useZodForm(() => import("@/lib/forms/schemas").then((m) => m.createAppSchema), …)`, — чтобы zod
// грузился отдельным чанком, а не в initial JS страниц (NFR-006, T-044). Компоненты импортируют
// отсюда только типы.

// F-1: правила — из контракта, плюс trim названия и проверка IANA-зоны.
export const createAppSchema = CreateAppBody.extend({
  name: zod.string().trim().pipe(CreateAppBody.shape.name),
  timezone: CreateAppBody.shape.timezone.refine(isTimezone),
});

// F-2: как F-1 — trim названия 1–64 и IANA-зона; платформа не меняется.
export const appSettingsSchema = zod.object({
  name: zod.string().trim().pipe(CreateAppBody.shape.name),
  timezone: zod.string().refine(isTimezone),
});

// F-3: метка write-ключа необязательна, до 64 символов; без платформы — ключ для любой платформы.
export const writeKeySchema = zod.object({
  label: zod.string().trim().max(createWriteKeyBodyLabelMax),
  platform: CreateWriteKeyBody.shape.platform,
});

// F-4: метка 1–64 после trim, хотя бы одно право; «Все приложения» — без app_id.
export const apiKeySchema = zod.object({
  label: zod.string().trim().pipe(CreateApiKeyBody.shape.label),
  scopes: CreateApiKeyBody.shape.scopes,
  app_id: zod.string(),
});

// F-5: chat_id — пусто или число (контракт и проверка в Postgres, OQ-059: @username не поддерживается),
// время дайджеста с шагом 15 минут.
export const telegramSchema = zod.object({
  telegram_chat_id: zod
    .string()
    .trim()
    .regex(/^(-?[0-9]{1,20})?$/),
  owner_timezone: zod.string().refine(isTimezone),
  digest_enabled: zod.boolean(),
  digest_time: zod.string().regex(/^([01][0-9]|2[0-3]):(00|15|30|45)$/),
  alerts_enabled: zod.boolean(),
});

// F-8: запрос поиска профиля обрезается по краям, 1–200 символов.
export const profileSearchSchema = zod.object({ q: zod.string().trim().min(1).max(200) });
