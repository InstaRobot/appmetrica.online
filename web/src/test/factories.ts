import { faker } from "@faker-js/faker";
import type {
  Error as ApiErrorBody,
  App,
  AppSummary,
  DeletionRequest,
  PortfolioRow,
  Profile,
  ProfileInstall,
  ProfileSession,
  ProfileSummary,
} from "@/lib/api/gen/model";

// Детерминированные фабрики для тестов компонентов: seed фиксирован, чтобы снапшоты и тексты не плавали.
faker.seed(20261013);

export function appSummary(overrides: Partial<AppSummary> = {}): AppSummary {
  return {
    id: faker.string.uuid(),
    name: "Notes iOS",
    platform: "ios",
    status: "active",
    ...overrides,
  };
}

export function app(overrides: Partial<App> = {}): App {
  return {
    id: faker.string.uuid(),
    name: "Notes iOS",
    platform: "ios",
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
