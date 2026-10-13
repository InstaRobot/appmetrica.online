// Команда выпуска ссылки регистрации passkey: S-01 setup-required и S-02 expired.
export const PASSKEY_LINK_COMMAND = "appmetrica admin passkey-link";

export function PasskeyLinkCommand() {
  return (
    <pre className="overflow-x-auto rounded-md border border-border bg-subtle px-3 py-2 text-left text-code">
      <code>{PASSKEY_LINK_COMMAND}</code>
    </pre>
  );
}
