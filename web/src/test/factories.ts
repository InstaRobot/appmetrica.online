import { faker } from "@faker-js/faker";
import type { Error as ApiErrorBody, App, AppSummary, PortfolioRow } from "@/lib/api/gen/model";

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

// Тело ответа-ошибки API в конверте `{"error": {...}}` (06-api-contracts.md §4).
export function errorBody(
  code: string,
  message = "Ошибка",
  requestId = "01JTESTREQUESTID0000000000",
): ApiErrorBody {
  return { error: { code, message, request_id: requestId } };
}
