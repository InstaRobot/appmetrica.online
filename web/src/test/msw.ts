import { setupServer } from "msw/node";
import { getAdminAppsMock } from "@/lib/api/gen/endpoints/admin-apps/admin-apps.msw";
import { getAdminAuthMock } from "@/lib/api/gen/endpoints/admin-auth/admin-auth.msw";
import { getAdminKeysMock } from "@/lib/api/gen/endpoints/admin-keys/admin-keys.msw";
import { getAdminPasskeysMock } from "@/lib/api/gen/endpoints/admin-passkeys/admin-passkeys.msw";
import { getAdminSettingsMock } from "@/lib/api/gen/endpoints/admin-settings/admin-settings.msw";
import { getAdminSystemMock } from "@/lib/api/gen/endpoints/admin-system/admin-system.msw";
import { getAppsMock } from "@/lib/api/gen/endpoints/apps/apps.msw";
import { getAudienceMock } from "@/lib/api/gen/endpoints/audience/audience.msw";
import { getDeletionMock } from "@/lib/api/gen/endpoints/deletion/deletion.msw";
import { getHealthMock } from "@/lib/api/gen/endpoints/health/health.msw";
import { getIntegrationMock } from "@/lib/api/gen/endpoints/integration/integration.msw";
import { getPortfolioMock } from "@/lib/api/gen/endpoints/portfolio/portfolio.msw";
import { getProfilesMock } from "@/lib/api/gen/endpoints/profiles/profiles.msw";
import { getRealtimeMock } from "@/lib/api/gen/endpoints/realtime/realtime.msw";
import { getRetentionMock } from "@/lib/api/gen/endpoints/retention/retention.msw";

// Хендлеры orval по умолчанию (faker-ответы 200) для всех тегов API. Тест переопределяет нужный
// эндпоинт через `server.use(getXxxMockHandler(override))` или `http.*` с ошибочным статусом.
export const defaultHandlers = [
  ...getAdminAppsMock(),
  ...getAdminAuthMock(),
  ...getAdminKeysMock(),
  ...getAdminPasskeysMock(),
  ...getAdminSettingsMock(),
  ...getAdminSystemMock(),
  ...getAppsMock(),
  ...getAudienceMock(),
  ...getDeletionMock(),
  ...getHealthMock(),
  ...getIntegrationMock(),
  ...getPortfolioMock(),
  ...getProfilesMock(),
  ...getRealtimeMock(),
  ...getRetentionMock(),
];

export const server = setupServer(...defaultHandlers);
