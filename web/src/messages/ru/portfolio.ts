import type { portfolio as en } from "../en/portfolio";

export const portfolio: typeof en = {
  title: "Портфель",
  addApp: "Добавить приложение",
  date: {
    previous: "Предыдущий день",
    next: "Следующий день",
    pick: "Выбрать дату: {date}",
  },
  noEvents: "За {date} событий нет. Проверьте вкладку «Интеграция» у приложений.",
  empty: {
    title: "Пока нет приложений",
    description:
      "Добавьте первое приложение — получите ключ и готовые сниппеты для отправки событий.",
  },
  showArchived: "Показать архивные ({count})",
  archive: {
    title: "Архив",
    name: "Название",
    platforms: "Платформы",
    archivedAt: "Дата архивации",
    actions: "Действия",
    open: "Открыть",
  },
  table: {
    caption: "Портфель приложений за {date}",
    columns: {
      name: "Приложение",
      platforms: "Платформы",
      today: "Сегодня",
      todayHint: "Неполный день: данные с 00:00 по TZ приложения",
      dau: "DAU {date}",
      delta: "Δ к {date}",
      new: "Новые",
      sessions: "Сессии",
      sparkline: "30 дней",
    },
    sort: {
      label: "Сортировка",
      descending: "По убыванию",
    },
    trend: {
      up: "Тренд за 30 дней: рост",
      down: "Тренд за 30 дней: падение",
      flat: "Тренд за 30 дней: без изменений",
    },
  },
  shares: {
    overlap: "Человек на нескольких платформах учтён в каждой",
  },
};
