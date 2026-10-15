// Тексты ошибок API по коду (14-ux-ui-spec.md §8.3, §14.3): коды — components.responses
// contracts/openapi/api.yaml, lib/errors.test.ts проверяет, что у каждого кода есть текст.
export const errors = {
  generic: "Something went wrong. Try again.",
  server: "Server error. Try again in a minute.",
  invalidRequestDetail: "Invalid request: {message}",
  rateLimitedRetry: "Too many requests. Retrying in {seconds} s.",
  codes: {
    invalid_request: "Invalid request.",
    range_too_large: "The period is too long: 366 days at most, 7 days for hourly detail.",
    unauthenticated: "Your session has expired. Sign in again.",
    insufficient_scope: "You don't have permission for this action.",
    csrf_check_failed: "The session is out of date. Refresh the page.",
    // Не код контракта: так отвечает статичный датасет демо-режима на любые изменения (lib/demo).
    demo_read_only: "Demo mode: changes are not available",
    app_not_found: "The app was not found or has been deleted",
    profile_not_found: "Profile not found",
    not_found: "The object was not found or has been deleted.",
    last_passkey: "You can't remove the last passkey — add another one first.",
    app_name_taken: "An app with this name already exists",
    confirm_name_mismatch: "The name doesn't match",
    deletion_pending:
      "The app is waiting for deletion: keys and restoring from the archive are unavailable",
    merge_in_progress: "The app is part of a merge — wait until it finishes",
    app_archived: "The target app is archived — restore it first",
    setup_token_expired: "The link has expired or has already been used",
    webauthn_verification_failed: "Couldn't verify the passkey. Try again or use another key.",
    rate_limited: "Too many requests. We'll retry automatically.",
    internal_error: "Server error. Try again in a minute.",
    storage_unavailable: "Data storage is temporarily unavailable. We'll retry automatically.",
    network_error: "No connection to the server. Check your connection.",
  },
};
