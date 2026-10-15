import type { realtime as en } from "../en/realtime";

export const realtime: typeof en = {
  title: "Realtime",
  pause: "Пауза",
  resume: "Продолжить",
  stale: "Не удаётся обновить данные. Показаны данные на {time}.",
  counter: {
    title: "Активны сейчас",
    window: "за последние 30 мин",
    updated: "обновлено {time}",
    paused: "на паузе",
    offline: "нет связи",
    announce: "Активны сейчас: {count}",
  },
  chart: {
    title: "Старты сессий по минутам",
    series: "Старты сессий",
    noData: "Нет данных за последние 30 минут",
    description: "Старты сессий по минутам, {from}–{to}: всего {total}, в последнюю минуту {last}",
  },
  feed: {
    title: "Последние события",
    columns: {
      time: "Время",
      event: "Событие",
      platform: "Платформа",
      version: "Версия",
      place: "Страна / город",
      install: "Install",
      person: "Человек",
    },
    install: "Установка {id}",
    profile: "Профиль {id}",
    empty: {
      title: "За последние 30 минут событий нет",
      description:
        "Отправьте тестовое событие из сниппета на вкладке «Интеграция» — оно появится здесь в течение 10 секунд.",
    },
  },
};
