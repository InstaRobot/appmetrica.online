// Заглушка T-019: реализация в следующем коммите.
export type PasskeyFlow = "login" | "setup";

export function supportsPasskeys(): boolean {
  return false;
}

export async function supportsPasskeyAutofill(): Promise<boolean> {
  return false;
}

export function deviceNameFromUserAgent(_userAgent: string): string {
  return "";
}

export async function loginWithPasskey(_opts: { autofill: boolean }): Promise<void> {
  throw new Error("not implemented");
}

export async function registerPasskeyBySetupLink(_token: string, _name: string): Promise<void> {
  throw new Error("not implemented");
}

export function cancelPasskeyCeremony(): void {}

export function isCeremonyAborted(_err: unknown): boolean {
  return false;
}

export function isSetupLinkExpired(_err: unknown): boolean {
  return false;
}

export function passkeyErrorMessage(_err: unknown, _flow: PasskeyFlow): string {
  return "";
}

export function validateDeviceName(_name: string): string | null {
  return null;
}
