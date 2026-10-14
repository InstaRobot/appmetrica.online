import { faker } from "@faker-js/faker";
import type {
  Error as ApiErrorBody,
  ApiKey,
  App,
  AppSummary,
  DeletionRequest,
  Passkey,
  PortfolioRow,
  Profile,
  ProfileInstall,
  ProfileSession,
  ProfileSummary,
  Settings,
  SystemStatus,
  WriteKey,
} from "@/lib/api/gen/model";

// Детерминированные фабрики для тестов компонентов: seed фиксирован, чтобы снапшоты и тексты не плавали.
faker.seed(20261013);

export function appSummary(overrides: Partial<AppSummary> = {}): AppSummary {
  return {
    id: faker.string.uuid(),
    name: "Notes iOS",
    platform: "ios",
    platforms: ["ios"],
    status: "active",
    ...overrides,
  };
}

export function app(overrides: Partial<App> = {}): App {
  return {
    id: faker.string.uuid(),
    name: "Notes iOS",
    platform: "ios",
    platforms: ["ios"],
    timezone: "Europe/Moscow",
    status: "active",
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    archived_at: null,
    ...overrides,
  };
}

export function portfolioRow(overrides: Partial<PortfolioRow> = {}): PortfolioRow {
  return {
    app: appSummary(),
    active_today: 5120,
    active_date: 12480,
    active_same_day_prev_week: 11988,
    active_delta_pct: 4.1,
    new_date: 1204,
    sessions_date: 31022,
    retention_d1: 0.38,
    retention_d7: 0.14,
    sparkline: Array.from({ length: 30 }, (_, i) => ({
      day: `2026-09-${String(i + 1).padStart(2, "0")}`,
      active: 12000 + i * 10,
    })),
    ...overrides,
  };
}

export function profileInstall(overrides: Partial<ProfileInstall> = {}): ProfileInstall {
  return {
    install_id: "5d8e2c1a-0b3f-4e5f-9a7b-0c1d2e3f4e5f",
    platform: "ios",
    os_name: "iOS",
    os_version: "26.0",
    app_version: "2.1.1",
    device_model: "iPhone17,1",
    device_type: "phone",
    locale: "ru-RU",
    country: "RU",
    city: "Москва",
    first_seen: "2026-03-03T09:00:00Z",
    last_seen: "2026-10-12T11:01:00Z",
    ...overrides,
  };
}

export function profileSession(overrides: Partial<ProfileSession> = {}): ProfileSession {
  return {
    session_id: faker.string.uuid(),
    install_id: "5d8e2c1a-0b3f-4e5f-9a7b-0c1d2e3f4e5f",
    started_at: "2026-10-12T11:01:00Z",
    ended_at: "2026-10-12T11:05:12Z",
    duration_ms: 252_000,
    app_version: "2.1.1",
    platform: "ios",
    environment: "production",
    country: "RU",
    city: "Москва",
    ...overrides,
  };
}

export function profile(overrides: Partial<Profile> = {}): Profile {
  return {
    person_key: "u:42",
    user_id: "42",
    first_seen: "2026-03-03T09:00:00Z",
    last_seen: "2026-10-12T11:01:00Z",
    installs: [profileInstall()],
    sessions: [profileSession()],
    next_before: null,
    ...overrides,
  };
}

export function profileSummary(overrides: Partial<ProfileSummary> = {}): ProfileSummary {
  return {
    person_key: "u:42",
    user_id: "42",
    matched_by: "user_id",
    installs_count: 2,
    first_seen: "2026-03-03T09:00:00Z",
    last_seen: "2026-10-12T11:01:00Z",
    ...overrides,
  };
}

export function deletionRequest(overrides: Partial<DeletionRequest> = {}): DeletionRequest {
  return {
    id: faker.string.uuid(),
    app_id: faker.string.uuid(),
    subject_type: "user",
    subject_value: "42",
    status: "pending",
    source: "dashboard",
    requested_at: "2026-10-12T12:00:00Z",
    started_at: null,
    completed_at: null,
    error: null,
    ...overrides,
  };
}

// Тело ответа-ошибки API в конверте `{"error": {...}}` (06-api-contracts.md §4).
export function errorBody(
  code: string,
  message = "Ошибка",
  requestId = "01JTESTREQUESTID0000000000",
): ApiErrorBody {
  return { error: { code, message, request_id: requestId } };
}

export function writeKey(overrides: Partial<WriteKey> = {}): WriteKey {
  return {
    id: faker.string.uuid(),
    app_id: faker.string.uuid(),
    key: "pk_7Hq2xxxxxxxxxxxxxxxxxxxxxxxxxAb0",
    label: "iOS 2.x",
    platform: null,
    created_at: "2026-10-01T09:00:00Z",
    revoked_at: null,
    last_used_at: "2026-10-12T11:30:00Z",
    ...overrides,
  };
}

export function apiKey(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: faker.string.uuid(),
    key_prefix: "sk_a8Kd2xQ1",
    label: "Telegram-отчёт",
    scopes: ["read"],
    app_id: null,
    created_at: "2026-10-01T09:00:00Z",
    revoked_at: null,
    last_used_at: null,
    ...overrides,
  };
}

export function passkey(overrides: Partial<Passkey> = {}): Passkey {
  return {
    id: faker.string.uuid(),
    name: "MacBook Pro",
    created_at: "2026-10-13T06:00:00Z",
    last_used_at: "2026-10-13T06:01:00Z",
    backup_eligible: true,
    backup_state: true,
    transports: ["internal", "hybrid"],
    ...overrides,
  };
}

export function settings(overrides: Partial<Settings> = {}): Settings {
  return {
    owner_timezone: "Europe/Moscow",
    telegram_chat_id: "123456789",
    digest_enabled: true,
    digest_time: "09:00",
    alerts_enabled: true,
    updated_at: "2026-10-13T06:00:00Z",
    ...overrides,
  };
}

export function systemStatus(overrides: Partial<SystemStatus> = {}): SystemStatus {
  return {
    clickhouse: { ok: true, disk_free_pct: 71.4, events_rows: 1204551902 },
    postgres: { ok: true },
    backup: { last_backup_at: "2026-10-13T00:12:00Z", last_backup_name: "increment-2026-10-13" },
    alerts: [
      {
        key: "disk_free",
        firing: false,
        first_fired_at: null,
        last_notified_at: null,
        last_value: "71.4",
      },
    ],
    jobs: [
      {
        id: faker.string.uuid(),
        job: "system_check",
        started_at: "2026-10-13T11:30:00Z",
        finished_at: "2026-10-13T11:30:01Z",
        status: "ok",
        details: {},
      },
    ],
    ...overrides,
  };
}
