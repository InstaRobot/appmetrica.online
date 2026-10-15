import type { profiles as en } from "../en/profiles";

export const profiles: typeof en = {
  title: "Профили",
  profileTitle: "Профиль",
  search: {
    label: "install_id или user_id",
    submit: "Найти",
    hint: "Поиск по точному совпадению. person_key вида u:… или i:… тоже подходит.",
    required: "Введите install_id или user_id",
    tooLong: "Не длиннее 200 символов",
  },
  results: {
    caption: "Результаты",
    columns: {
      person: "Человек",
      userId: "user_id",
      installs: "Устройств",
      firstSeen: "Первый визит",
      lastSeen: "Последний",
    },
    profileLink: "Профиль {key}",
    emptyTitle: "Ничего не найдено",
    emptyDescription:
      "Проверьте значение: поиск работает только по точному совпадению install_id или user_id. Данные старше 25 месяцев удаляются.",
  },
  notFound: {
    title: "Профиль не найден",
    description: "Возможно, данные удалены или ещё не поступили.",
  },
  backToSearch: "К поиску",
  installsTitle: "Устройства",
  sessionsTitle: "Сессии",
  deleted: "Данные удалены",
  card: {
    copyKey: "Копировать person_key",
    deleteData: "Удалить данные",
    firstSeen: "Первый визит:",
    lastSeen: "Последний:",
    deletionStatus: "Статус удаления",
    deletionStatusLabel: "Статус удаления:",
  },
  installs: {
    caption: "Устройства",
    columns: {
      installId: "Install ID",
      platform: "Платформа",
      device: "Устройство",
      os: "ОС",
      version: "Версия",
      geo: "Гео",
      firstSeen: "Первый",
      lastSeen: "Последний",
    },
  },
  sessions: {
    empty: "Сессий нет",
    caption: "Сессии",
    columns: {
      start: "Начало",
      duration: "Длительность",
      version: "Версия",
      platform: "Платформа",
      install: "Install",
    },
    noEnd: "— (нет session_end)",
    more: "Показать ещё",
  },
  deleteDialog: {
    title: "Удалить все данные этого человека?",
    description:
      "Будут удалены все события и связки устройств для {key} в приложении «{app}». Агрегаты закрытых дней не меняются. Удаление выполняется в фоне до 24 часов и необратимо.",
    confirmText: "УДАЛИТЬ",
    confirm: "Удалить данные",
    created: "Запрос на удаление создан",
  },
  deletionStatus: {
    pending: "В очереди",
    running: "Выполняется",
    done: "Удалено",
    failed: "Ошибка",
  },
};
