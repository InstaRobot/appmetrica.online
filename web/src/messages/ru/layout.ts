import type { layout as en } from "../en/layout";

export const layout: typeof en = {
  language: {
    label: "Язык",
    switchTo: "Переключить на {language}",
  },
  publicHeader: {
    home: "appmetrica.online — на главную",
    docs: "Документация",
    signIn: "Войти",
  },
  header: {
    home: "appmetrica.online — в портфель",
    nav: "Основная навигация",
    portfolio: "Портфель",
    docs: "Документация",
    settings: "Настройки",
  },
  userMenu: {
    label: "Меню пользователя",
    signOut: "Выйти",
  },
  appSwitcher: {
    label: "Приложение",
    allApps: "Все приложения",
    search: "Найти приложение…",
    notFound: "Ничего не найдено",
    empty: "Приложений пока нет",
    add: "+ Добавить приложение",
  },
  appTabs: {
    label: "Разделы приложения",
    audience: "Аудитория",
    retention: "Retention",
    realtime: "Realtime",
    profiles: "Профили",
    integration: "Интеграция",
    settings: "Настройки",
  },
  appShell: {
    notFoundHint: "Проверьте адрес или выберите приложение в портфеле.",
    toPortfolio: "В портфель",
  },
  footer: {
    docs: "Документация",
  },
  skipLink: "Перейти к содержимому",
  theme: {
    dark: "Включить тёмную тему",
    light: "Включить светлую тему",
  },
};
