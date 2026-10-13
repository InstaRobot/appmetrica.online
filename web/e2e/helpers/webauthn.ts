import type { Page } from "@playwright/test";

export type VirtualAuthenticator = {
  authenticatorId: string;
  // Автоподтверждение присутствия. Выключенное держит церемонии в ожидании: так conditional UI не завершает
  // вход раньше клика по кнопке (виртуальный аутентификатор Chrome подтверждает и автозаполнение).
  setUserPresence(enabled: boolean): Promise<void>;
};

// Виртуальный WebAuthn-аутентификатор Chrome DevTools Protocol (22-test-strategy.md §19.2): платформенный
// ключ с resident key и user verification. Живёт, пока жива страница.
export async function addVirtualAuthenticator(page: Page): Promise<VirtualAuthenticator> {
  const client = await page.context().newCDPSession(page);
  await client.send("WebAuthn.enable");
  const { authenticatorId } = await client.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });
  return {
    authenticatorId,
    async setUserPresence(enabled) {
      await client.send("WebAuthn.setAutomaticPresenceSimulation", { authenticatorId, enabled });
    },
  };
}
