import type { FullConfig } from "@playwright/test";
import { saveOwnerSession } from "./helpers/seed";

// Один владелец с passkey на весь прогон: cookie сохраняется в e2e/.auth/owner.json.
export default async function globalSetup(config: FullConfig): Promise<void> {
  const baseURL = config.projects[0]?.use.baseURL ?? "http://localhost:3000";
  await saveOwnerSession(baseURL);
}
