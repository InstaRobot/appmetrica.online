// Вход и регистрация passkey: S-01 /login, S-02 /setup/[token] (components/auth, lib/webauthn).
export const auth = {
  login: {
    metaTitle: "Sign in",
    title: "Sign in to appmetrica.online",
    hint: "Use a passkey on this device or your phone.",
    // {command} — команда выпуска ссылки в <code>.
    setupRequired:
      "No passkey is set up yet. Run {command} on the server and open the link it prints.",
  },
  setup: {
    metaTitle: "Passkey setup",
    title: "Set up a passkey",
    oneTime: "This link works once and expires in 15 minutes.",
    defaultDeviceName: "Device",
    deviceName: "Device name",
    submit: "Create passkey",
    submitting: "Creating passkey…",
    success: "Passkey created. Opening the dashboard…",
    expiredTitle: "This link has expired or has already been used",
    expiredHint: "Get a new one: run {command} on the server.",
  },
  passkey: {
    signIn: "Sign in with a passkey",
    waiting: "Waiting for confirmation…",
    unsupported: "Your browser doesn't support passkeys",
    help: "Where passkeys work",
  },
  deviceName: {
    required: "Enter a device name",
    tooLong: "64 characters at most",
  },
  passkeyErrors: {
    login: "Couldn't sign in. The confirmation was canceled or the key didn't match. Try again.",
    setup: "Couldn't create the passkey. Try again.",
    network: "No connection to the server. Check your connection.",
    rateLimited: "Too many attempts. Try again in a minute.",
    sessionExpired: "The session is out of date. Refresh the page.",
  },
};
