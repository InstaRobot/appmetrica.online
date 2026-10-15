// Тексты демо-датасета (lib/demo): ответы демо-API на ошибки запроса, подписи ключей, сообщения отказов
// и ошибок, которые в настоящем режиме пришли бы от API. Демо-слой работает в браузере и импортирует
// только этот модуль и его русскую пару (lib/demo/texts.ts), а не весь словарь.
export const demoData = {
  errors: {
    readOnly: "Demo mode: changes are not available",
    notFound: "Not found",
    appNotFound: "App not found.",
    profileNotFound: "Profile not found.",
    oneOf: "{name}: allowed values are {allowed}.",
    integer: "{name}: an integer from {min} to {max}.",
    boolean: "{name}: allowed values are true or false.",
    required: "The {name} parameter is required.",
    isoDate: "{name}: expected a YYYY-MM-DD date.",
    platform: "platform: unknown platform.",
    maxLength: "{name}: {max} characters at most.",
    country: "country: an ISO 3166-1 alpha-2 country code of two uppercase Latin letters.",
    fromAfterTo: "from can't be later than to.",
    rangeTooLarge: "The range can't exceed {max} days ({days} requested).",
    hourRangeTooLarge: "The range for granularity=hour can't exceed {max} days ({days} requested).",
    uuid: "{name}: expected a UUID.",
    searchLength: "q: from 1 to 128 characters.",
    personKey: "person_key: expected u:<user_id> or i:<install_id>.",
    before: "before: expected an RFC 3339 date-time.",
    mergeSelf: "You can't merge an app with itself.",
  },
  // Метки write-ключей, как при создании приложения (PLATFORM_LABELS в components/common/platform-icon.tsx).
  writeKeyLabels: {
    ios: "iOS",
    android: "Android",
    web: "Web",
    macos: "macOS",
    windows: "Windows",
    linux: "Linux",
    bot: "Bot",
    server: "Backend",
    other: "Other",
  },
  leakedWriteKey: "Old build (leaked into a log)",
  apiKeyLabels: {
    reconcile: "AppMetrica reconciliation",
    gdpr: "On-request data deletion",
    cron: "Old cron report",
  },
  deletionTimeout: "ClickHouse: timeout after 5 attempts",
  // Примеры сообщений отказов ingest на экране интеграции (S-10).
  rejections: {
    duration_required: "session_end requires the duration_ms field (0..86400000).",
    event_too_old: "The event is older than 7 days after clock correction.",
    invalid_field: "timestamp must be in RFC 3339 format with an offset.",
    missing_field: "The session_id field is required.",
    install_rate_limited: "Too many events from this device. Try again later.",
    user_id_looks_like_pii: "user_id looks like an email or phone number. Send an opaque ID.",
    unknown_event_name: "Allowed names: session_start, session_end.",
    app_archived: "The app is archived, event intake has stopped.",
    app_version_not_string: "context.app_version must be a string.",
  },
};
