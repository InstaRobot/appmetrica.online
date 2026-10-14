// Статичные данные Admin API демо-режима: настройки, статус системы, passkeys, ключи и журнал удалений.
// Ключи явно демонстрационные (pk_demo…, у sk_ — только key_prefix), времена недавних событий
// отсчитываются от now.
import type {
  ApiKeyList,
  AuthState,
  DeletionRequest,
  DeletionRequestList,
  HealthStatus,
  JobRun,
  MergePreview,
  PasskeyList,
  Platform,
  ReadyStatus,
  Settings,
  SystemStatus,
  WriteKey,
  WriteKeyList,
} from "@/lib/api/gen/model";
import { DEMO_APP_IDS, DEMO_APPS, DEMO_OWNER_TIMEZONE } from "./apps";
import { type DemoApp, dayStats, demoApp, makeScope, segmentOf } from "./model";
import { alnumFrom, uuidFrom } from "./random";
import { DAY_MS, eachDay, HOUR_MS, isoUtc, localTime, MINUTE_MS, zonedMidnight } from "./time";

export const AUTH_STATE: AuthState = { authenticated: true, setup_required: false };
export const HEALTH: HealthStatus = { status: "ok" };
export const READY: ReadyStatus = {
  status: "ready",
  checks: { clickhouse: "ok", postgres: "ok" },
};

export const SETTINGS: Settings = {
  owner_timezone: DEMO_OWNER_TIMEZONE,
  telegram_chat_id: "-1001234567890",
  digest_enabled: true,
  digest_time: "09:00",
  alerts_enabled: true,
  updated_at: "2026-10-01T07:05:12.000Z",
};

export function passkeys(now: number): PasskeyList {
  return {
    items: [
      {
        id: "0199d5e1-7a10-7b20-8c30-4d405e607080",
        name: "MacBook Pro (Touch ID)",
        created_at: "2025-03-17T08:05:10.000Z",
        last_used_at: isoUtc(now - 3 * HOUR_MS - 14 * MINUTE_MS),
        backup_eligible: true,
        backup_state: true,
        transports: ["internal", "hybrid"],
      },
      {
        id: "0199d5e2-8b21-7c31-9d41-5e516f708192",
        name: "iPhone",
        created_at: "2025-03-17T08:09:44.000Z",
        last_used_at: isoUtc(now - 4 * DAY_MS - 2 * HOUR_MS),
        backup_eligible: true,
        backup_state: true,
        transports: ["internal", "hybrid"],
      },
      {
        id: "0199d5e3-9c32-7d42-ae52-6f6270819203",
        name: "YubiKey 5C",
        created_at: "2025-04-01T12:30:00.000Z",
        last_used_at: null,
        backup_eligible: false,
        backup_state: false,
        transports: ["usb", "nfc"],
      },
    ],
  };
}

// demoWriteKey — pk_ из 32 символов, начинается с «demo»: видно, что ключ ненастоящий.
function demoWriteKey(appId: string, n: number): string {
  return `pk_demo${alnumFrom(28, appId, "write-key", n)}`;
}

// mergePreview — предпросмотр слияния source в target: события — начало и конец каждой сессии источника
// за всю его историю (оба окружения), ключи — активные ключи источника.
export function mergePreview(target: DemoApp, source: DemoApp, now: number): MergePreview {
  let events = 0;
  let firstDay: string | null = null;
  let lastDay: string | null = null;
  for (const env of ["production", "development"] as const) {
    const scope = makeScope(source, now, { env });
    for (const day of eachDay(source.firstDay, source.lastDay ?? scope.today)) {
      const sessions = dayStats(scope, day).sessions;
      if (sessions === 0) continue;
      events += sessions * 2;
      if (firstDay === null || day < firstDay) firstDay = day;
      if (lastDay === null || day > lastDay) lastDay = day;
    }
  }
  const { id, name, platform, platforms, status, timezone } = source.app;
  return {
    source: { id, name, platform, platforms, status },
    events,
    first_day: firstDay,
    last_day: lastDay,
    write_keys: writeKeys(id, now).items.filter((k) => k.revoked_at === null).length,
    timezone_differs: timezone !== target.app.timezone,
    target_timezone: target.app.timezone,
    source_timezone: timezone,
  };
}

// Подписи ключей, как при создании приложения (PLATFORM_LABELS в components/common/platform-icon.tsx).
const KEY_LABELS: Readonly<Record<Platform, string>> = {
  ios: "iOS",
  android: "Android",
  web: "Веб",
  macos: "macOS",
  windows: "Windows",
  linux: "Linux",
  bot: "Бот",
  server: "Бэкенд",
  other: "Другое",
};

// writeKeys — ключ на каждую платформу приложения; ключ платформы, добавленной позже, создан за два дня
// до её запуска. У мобильных приложений есть и отозванный ключ без платформы.
export function writeKeys(appId: string, now: number): WriteKeyList {
  const demo = demoApp(appId);
  if (!demo) return { items: [] };
  const { app } = demo;
  const items: WriteKey[] = app.platforms.map((platform, i) => {
    const launch = segmentOf(demo, platform)?.launch ?? demo.firstDay;
    const created =
      launch > demo.firstDay
        ? isoUtc(zonedMidnight(launch, app.timezone) - 2 * DAY_MS + 9 * HOUR_MS + 15 * MINUTE_MS)
        : app.created_at;
    return {
      id: uuidFrom(appId, "write-key", i === 0 ? 1 : platform),
      app_id: appId,
      key: demoWriteKey(appId, i === 0 ? 1 : 3 + i),
      label: KEY_LABELS[platform],
      platform,
      created_at: created,
      revoked_at: null,
      last_used_at: app.archived_at ?? isoUtc(now - 1_800 - i * 700),
    };
  });
  if (app.platforms.includes("ios") || app.platforms.includes("android")) {
    items.push({
      id: uuidFrom(appId, "write-key", 2),
      app_id: appId,
      key: demoWriteKey(appId, 2),
      label: "Старая сборка (утёк в лог)",
      platform: null,
      created_at: app.created_at,
      revoked_at: "2026-02-11T16:20:00.000Z",
      last_used_at: "2026-02-11T16:18:41.000Z",
    });
  }
  return { items };
}

export function apiKeys(now: number): ApiKeyList {
  return {
    items: [
      {
        id: "0199d5f8-1111-7aaa-8bbb-0c0d0e0f1011",
        key_prefix: "sk_demoR3ad",
        label: "Сверка с AppMetrica",
        scopes: ["read"],
        app_id: null,
        created_at: "2025-05-04T10:00:00.000Z",
        revoked_at: null,
        last_used_at: isoUtc(now - 6 * HOUR_MS - 3 * MINUTE_MS),
      },
      {
        id: "0199d5f8-2222-7bbb-9ccc-1d1e1f202122",
        key_prefix: "sk_demoGdpr",
        label: "Удаление данных по запросу",
        scopes: ["read", "delete"],
        app_id: DEMO_APP_IDS.notes,
        created_at: "2025-09-12T14:30:00.000Z",
        revoked_at: null,
        last_used_at: isoUtc(now - 2 * DAY_MS - 3 * HOUR_MS),
      },
      {
        id: "0199d5f8-3333-7ccc-addd-2e2f30313233",
        key_prefix: "sk_demoCron",
        label: "Старый cron-отчёт",
        scopes: ["read"],
        app_id: null,
        created_at: "2025-03-20T09:00:00.000Z",
        revoked_at: "2026-01-15T11:00:00.000Z",
        last_used_at: "2026-01-15T03:00:02.000Z",
      },
    ],
  };
}

// lastAt — последний момент локального времени hour:minute в owner TZ не позже now.
function lastAt(now: number, hour: number, minute: number): number {
  const today = localTime(now, DEMO_OWNER_TIMEZONE).date;
  const t = zonedMidnight(today, DEMO_OWNER_TIMEZONE) + hour * HOUR_MS + minute * MINUTE_MS;
  return t <= now ? t : t - DAY_MS;
}

function job(name: JobRun["job"], started: number, ms: number, details: JobRun["details"]): JobRun {
  return {
    id: uuidFrom("job", name, started),
    job: name,
    started_at: isoUtc(started),
    finished_at: isoUtc(started + ms),
    status: "ok",
    details,
  };
}

export function systemStatus(now: number): SystemStatus {
  const backup = lastAt(now, 2, 0);
  const minute = Math.floor(now / MINUTE_MS) * MINUTE_MS;
  const closeDay = lastAt(now, 0, 10);
  return {
    clickhouse: { ok: true, disk_free_pct: 78.6, events_rows: 184_233_901 },
    postgres: { ok: true },
    backup: {
      last_backup_at: isoUtc(backup + 41_000),
      last_backup_name: `increment-${isoUtc(backup).slice(0, 19).replaceAll(":", "-")}`,
    },
    alerts: (
      [
        "disk_free",
        "insert_errors",
        "backup_stale",
        "ingest_down",
        "api_down",
        "clickhouse_down",
      ] as const
    ).map((key) => ({
      key,
      firing: false,
      first_fired_at: null,
      last_notified_at: key === "disk_free" ? "2026-08-21T04:00:00.000Z" : null,
      last_value: key === "disk_free" ? "78.6" : "0",
    })),
    jobs: [
      job("close_day", closeDay, 4_812, {
        day: isoUtc(closeDay - DAY_MS).slice(0, 10),
        apps: DEMO_APPS.length,
      }),
      job("digest", lastAt(now, 9, 0), 1_204, { sent: true }),
      job("system_check", minute - (minute % (5 * MINUTE_MS)), 1_412, { checks: 5 }),
      job("deletion", minute, 214, { processed: 0 }),
      job("session_cleanup", lastAt(now, 3, 30), 96, { deleted: 2 }),
    ],
  };
}

const DELETIONS: readonly (Omit<DeletionRequest, "requested_at" | "started_at" | "completed_at"> & {
  ago: number;
  runMs: number | null;
  doneMs: number | null;
})[] = [
  {
    id: "0199d6d0-1111-7aaa-8bbb-0c0d0e0f1001",
    app_id: DEMO_APP_IDS.bot,
    subject_type: "user",
    subject_value: "tg_7a1c9e04b2",
    status: "pending",
    source: "api",
    error: null,
    ago: 40_000,
    runMs: null,
    doneMs: null,
  },
  {
    id: "0199d6d0-2222-7bbb-9ccc-1d1e1f201002",
    app_id: DEMO_APP_IDS.notes,
    subject_type: "install",
    subject_value: "5a7c9e1b-3d5f-4a7c-9e1b-3d5f7a9c1e3b",
    status: "running",
    source: "dashboard",
    error: null,
    ago: 3 * MINUTE_MS,
    runMs: 2 * MINUTE_MS + 40_000,
    doneMs: null,
  },
  {
    id: "0199d6d0-3333-7ccc-addd-2e2f30311003",
    app_id: DEMO_APP_IDS.notes,
    subject_type: "user",
    subject_value: "a7c4e1b9-0d2f-4e6a-8b3c-5d7e9f1a2b4c",
    status: "done",
    source: "api",
    error: null,
    ago: 2 * DAY_MS + 3 * HOUR_MS,
    runMs: 31_120,
    doneMs: 44_871,
  },
  {
    id: "0199d6d0-4444-7ddd-beee-3f4041421004",
    app_id: DEMO_APP_IDS.notes,
    subject_type: "install",
    subject_value: "f1e2d3c4-b5a6-4978-8a6b-5c4d3e2f1a0b",
    status: "done",
    source: "dashboard",
    error: null,
    ago: 5 * DAY_MS + 7 * HOUR_MS,
    runMs: 18_402,
    doneMs: 26_995,
  },
  {
    id: "0199d6d0-5555-7eee-8fff-404142431005",
    app_id: DEMO_APP_IDS.site,
    subject_type: "user",
    subject_value: "site-1187",
    status: "failed",
    source: "api",
    error: "ClickHouse: timeout после 5 попыток",
    ago: 9 * DAY_MS + 2 * HOUR_MS,
    runMs: 44_000,
    doneMs: 4 * HOUR_MS + 12_000,
  },
];

export function deletionRequests(now: number, appId: string | null): DeletionRequestList {
  const items: DeletionRequest[] = DELETIONS.filter(
    (d) => appId === null || d.app_id === appId,
  ).map(({ ago, runMs, doneMs, ...d }) => {
    const requested = now - ago;
    return {
      ...d,
      requested_at: isoUtc(requested),
      started_at: runMs === null ? null : isoUtc(requested + runMs),
      completed_at: doneMs === null ? null : isoUtc(requested + doneMs),
    };
  });
  return { items };
}
