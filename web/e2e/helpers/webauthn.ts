import type { Page } from "@playwright/test";

// Виртуальный WebAuthn-аутентификатор Chrome DevTools Protocol (22-test-strategy.md §19.2): платформенный
// ключ с resident key и user verification, подтверждение присутствия — автоматически. Живёт, пока жива страница.
export async function addVirtualAuthenticator(page: Page): Promise<{ authenticatorId: string }> {
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
  return { authenticatorId };
}
