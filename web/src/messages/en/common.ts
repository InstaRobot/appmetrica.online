// Общие слова интерфейса: кнопки и состояния, которые повторяются на разных экранах, и тексты общих
// компонентов (components/common, components/feedback, components/ui, lib/clipboard).
export const common = {
  loading: "Loading…",
  loadingCalendar: "Loading calendar…",
  close: "Close",
  cancel: "Cancel",
  save: "Save",
  retry: "Try again",
  copy: {
    copied: "Copied",
    failed: "Couldn't copy",
    code: "Copy code",
    value: "Copy {label}",
  },
  timezone: {
    placeholder: "Select a time zone",
    search: "Search time zones…",
    notFound: "Nothing found",
  },
  // Подписи платформ приложения и устройства (enum Platform контракта).
  platforms: {
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
  archived: "This app is archived: its events are rejected with the app_archived code.",
  confirmDelete: {
    type: "Type {text}",
    deleting: "Deleting…",
  },
};
