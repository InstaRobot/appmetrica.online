import type { common as en } from "../en/common";

export const common: typeof en = {
  loading: "Загрузка…",
  loadingCalendar: "Загрузка календаря…",
  close: "Закрыть",
  cancel: "Отмена",
  save: "Сохранить",
  retry: "Повторить",
  copy: {
    copied: "Скопировано",
    failed: "Не удалось скопировать",
    code: "Копировать код",
    value: "Копировать {label}",
  },
  timezone: {
    placeholder: "Выберите часовой пояс",
    search: "Найти часовой пояс…",
    notFound: "Ничего не найдено",
  },
  platforms: {
    ios: "iOS",
    android: "Android",
    web: "Веб",
    macos: "macOS",
    windows: "Windows",
    linux: "Linux",
    bot: "Бот",
    server: "Бэкенд",
    other: "Другое",
  },
  archived: "Приложение в архиве: события отклоняются с кодом app_archived.",
  confirmDelete: {
    type: "Введите {text}",
    deleting: "Удаляем…",
  },
};
