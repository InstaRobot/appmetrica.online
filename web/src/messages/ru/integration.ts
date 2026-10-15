import type { integration as en } from "../en/integration";

export const integration: typeof en = {
  snippetComments: {
    expectedResponse: "Ожидаемый ответ",
    swiftSessionStart:
      "Старт сессии при выходе на передний план. InstallID.current — UUID из Keychain",
    swiftRecipe: "(рецепт хранения и офлайн-очередь: /docs/recipes/ios-swift).",
    kotlinThread: "Вызывать из фонового потока (Dispatchers.IO). Возвращает HTTP-код ответа.",
    goIp: 'для s2s: передайте IP пользователя или "none"',
    goInstallId: "для бота — стабильный UUID на пользователя чата",
  },
  rejectionCodes: {
    missing_field: {
      description: "Нет обязательного поля",
      hint: "В каждом событии должны быть insert_id, name, timestamp, install_id, session_id",
    },
    invalid_field: {
      description: "Неверный формат поля",
      hint: "UUID — 36 символов; время — RFC 3339 с часовым поясом, например 2026-10-13T09:00:00Z",
    },
    unknown_event_name: {
      description: "Неизвестное имя события",
      hint: "Сейчас принимаются только session_start и session_end",
    },
    event_too_old: {
      description: "Событие старше 7 дней",
      hint: "Удаляйте из офлайн-очереди события старше 7 дней",
    },
    user_id_looks_like_pii: {
      description: "user_id похож на email или телефон",
      hint: "Передавайте непрозрачный внутренний ID (UUID из вашего бэкенда); Telegram ID — через HMAC",
    },
    duration_required: {
      description: "У session_end нет duration_ms",
      hint: "Передавайте длительность сессии в миллисекундах",
    },
    install_rate_limited: {
      description: "Превышен лимит событий для одного install_id",
      hint: "Проверьте, не отправляет ли клиент события в цикле; такие события можно повторить позже",
    },
    invalid_json: {
      description: "Тело запроса — не JSON",
      hint: "Проверьте сериализацию и Content-Encoding",
    },
    invalid_batch: {
      description: "Неверная структура батча",
      hint: "Нужны sent_at и от 1 до 500 событий",
    },
    unsupported_content_encoding: {
      description: "Неподдерживаемое сжатие",
      hint: "Используйте gzip или отправляйте без сжатия",
    },
    payload_too_large: {
      description: "Батч больше 1 МБ",
      hint: "Разбейте батч на части",
    },
    unsupported_media_type: {
      description: "Неверный Content-Type",
      hint: "Используйте application/json или text/plain",
    },
    app_archived: {
      description: "Приложение в архиве",
      hint: "Верните приложение из архива в настройках",
    },
  },
};
