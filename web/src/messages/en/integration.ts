// Пространство имён «integration»: вкладка «Интеграция» (S-10), сниппеты и коды отказов ingest.
export const integration = {
  title: "Integration",
  showKeys: "Show key and snippets",
  kpi: {
    accepted: "Accepted, last {hours}h",
    rejected: "Rejected, last {hours}h",
    rejectionRate: "Rejection rate",
    lastEvent: "Last event",
  },
  sections: {
    hourly: "By hour",
    reasons: "Rejection reasons",
    versions: "By app version",
  },
  rateWarning: "Rejection rate of 1% or higher",
  endpoint: {
    title: "Endpoint",
    hint: "One for all platforms of the app",
    field: "endpoint",
  },
  addPlatform: "Add platform",
  platform: {
    status: "{platform} status",
    received: "First event received",
    waiting: "Waiting for the first event from {platform}",
    writeKeyHint: "Write key (public, embedded in the app)",
    writeKeyField: "write key",
    noKey: "No active key for {platform}.",
    createKey: "Create a key for {platform}",
  },
  firstEvent: {
    label: "First event",
    toast: "First event received",
    receivedAt: "First event received at {time}",
    connected: "— {app} is connected!",
    waiting: "Waiting for the first event…",
    polling: "Checking every 3 seconds.",
    openAudience: "Open Audience",
  },
  reasonsTable: {
    none: "No rejections in the last 24 hours ✓",
    code: "Code",
    description: "Description",
    count: "Count",
    last: "Last",
    example: "Example",
    examplePayload: "Example payload: {code}",
    examplePayloadJson: "Example payload as JSON",
  },
  versionsTable: {
    none: "No events in the last 24 hours",
    version: "Version",
    accepted: "Accepted",
    rejected: "Rejected",
    rejectionRate: "Rejection rate",
    lastEvent: "Last event",
    notSet: "(not set)",
  },
  chart: {
    accepted: "Accepted",
    rejected: "Rejected",
    empty: "No data for the last 24 hours",
    description: "Events by hour, {from}–{to}: {accepted} accepted, {rejected} rejected",
  },
  snippets: {
    tabs: "Snippet language",
    code: "Code example in {lang}",
    recipes: "Full recipes with an offline queue and sessions are in the docs →",
  },
  // Комментарии внутри сниппетов онбординга (lib/snippets.ts); сам код сниппетов не переводится.
  snippetComments: {
    expectedResponse: "Expected response",
    swiftSessionStart:
      "Start a session when the app comes to the foreground. InstallID.current is a UUID from the Keychain",
    swiftRecipe: "(storage recipe and offline queue: /docs/recipes/ios-swift).",
    kotlinThread: "Call from a background thread (Dispatchers.IO). Returns the HTTP response code.",
    goIp: 'for s2s: pass the user\'s IP or "none"',
    goInstallId: "for a bot, a stable UUID per chat user",
  },
  // Коды отказов ingest (14-ux-ui-spec.md §14.3): семь RejectionCode событий и batch-коды.
  rejectionCodes: {
    missing_field: {
      description: "Required field is missing",
      hint: "Every event needs insert_id, name, timestamp, install_id, session_id",
    },
    invalid_field: {
      description: "Invalid field format",
      hint: "A UUID is 36 characters; time is RFC 3339 with a time zone, e.g. 2026-10-13T09:00:00Z",
    },
    unknown_event_name: {
      description: "Unknown event name",
      hint: "Only session_start and session_end are accepted for now",
    },
    event_too_old: {
      description: "Event older than 7 days",
      hint: "Drop events older than 7 days from the offline queue",
    },
    user_id_looks_like_pii: {
      description: "user_id looks like an email or a phone number",
      hint: "Send an opaque internal ID (a UUID from your backend); pass a Telegram ID through HMAC",
    },
    duration_required: {
      description: "session_end has no duration_ms",
      hint: "Send the session duration in milliseconds",
    },
    install_rate_limited: {
      description: "Event limit exceeded for a single install_id",
      hint: "Check that the client isn't sending events in a loop; these events can be retried later",
    },
    invalid_json: {
      description: "Request body isn't JSON",
      hint: "Check serialization and Content-Encoding",
    },
    invalid_batch: {
      description: "Invalid batch structure",
      hint: "A batch needs sent_at and 1 to 500 events",
    },
    unsupported_content_encoding: {
      description: "Unsupported compression",
      hint: "Use gzip or send uncompressed",
    },
    payload_too_large: {
      description: "Batch larger than 1 MB",
      hint: "Split the batch into smaller parts",
    },
    unsupported_media_type: {
      description: "Invalid Content-Type",
      hint: "Use application/json or text/plain",
    },
    app_archived: {
      description: "App is archived",
      hint: "Restore the app from the archive in its settings",
    },
  },
};
