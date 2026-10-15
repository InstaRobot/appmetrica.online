// Пространство имён «integration»: вкладка «Интеграция» (S-10), сниппеты и коды отказов ingest.
export const integration = {
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
