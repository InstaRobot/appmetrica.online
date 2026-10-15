// Приложения демо-режима: фиксированные UUID, имена и порядок (ID «Notes» совпадает с примерами
// 06-api-contracts.md, поэтому curl-рецепты документации открывают то же приложение).
import type { App } from "@/lib/api/gen/model";

export const DEMO_APP_IDS = {
  notes: "0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f",
  site: "0199d5f0-3c4d-7e5f-a061-7c8d9e0f1a2b",
  bot: "0199d5f0-4d5e-7f60-b172-8d9e0f1a2b3c",
  oldGame: "0199d5f0-5e6f-7071-8283-9e0f1a2b3c4d",
} as const;

// owner_timezone демо-настроек: дата портфеля по умолчанию — вчера в этой зоне.
export const DEMO_OWNER_TIMEZONE = "Europe/Moscow";

export const DEMO_APPS: readonly App[] = [
  {
    id: DEMO_APP_IDS.notes,
    name: "Notes",
    platform: "ios",
    platforms: ["ios", "android", "web"],
    timezone: "Europe/Moscow",
    status: "active",
    created_at: "2025-03-17T08:12:44.000Z",
    updated_at: "2026-08-12T09:15:00.000Z",
    archived_at: null,
  },
  {
    id: DEMO_APP_IDS.site,
    name: "site.ru",
    platform: "web",
    platforms: ["web"],
    timezone: "Europe/Moscow",
    status: "active",
    created_at: "2025-06-10T07:30:02.000Z",
    updated_at: "2025-06-10T07:30:02.000Z",
    archived_at: null,
  },
  {
    id: DEMO_APP_IDS.bot,
    name: "TG Helper",
    platform: "bot",
    platforms: ["bot"],
    timezone: "Asia/Yekaterinburg",
    status: "active",
    created_at: "2025-11-20T06:48:31.000Z",
    updated_at: "2026-03-14T09:12:00.000Z",
    archived_at: null,
  },
  {
    id: DEMO_APP_IDS.oldGame,
    name: "Old Game",
    platform: "ios",
    platforms: ["ios"],
    timezone: "UTC",
    status: "archived",
    created_at: "2024-09-02T13:00:00.000Z",
    updated_at: "2026-06-30T15:20:00.000Z",
    archived_at: "2026-06-30T15:20:00.000Z",
  },
];
