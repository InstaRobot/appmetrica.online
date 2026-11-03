import type { events as en } from "../en/events";

export const events: typeof en = {
  title: "События",
  table: {
    label: "Пользовательские события",
    name: "Событие",
    events: "События",
    people: "Люди",
    installs: "Установки",
    delta: "Изменение",
    deltaHint: "Изменение числа событий к предыдущему периоду такой же длины",
    perPerson: "На человека",
    perPersonHint: "События, делённые на людей",
  },
  truncated: "Показаны первые 500 событий. Сузьте период или фильтры.",
  empty: {
    title: "Пользовательских событий пока нет",
    description:
      "Здесь появятся события со своим именем, которые отправляет приложение. Системные события вроде session_start в список не входят.",
    action: "Как отправлять события",
  },
  event: {
    back: "События",
    notFound: {
      title: "Событие не найдено",
      description:
        "Такого пользовательского события у приложения нет. Системные события вроде session_start здесь не показываются.",
      action: "К списку событий",
    },
    totals: "Итоги события",
    kpi: {
      events: "События",
      people: "Люди",
      installs: "Установки",
      perPerson: "Событий на человека",
      perPersonHint: "События, делённые на людей",
    },
    chart: {
      metric: "Метрика графика",
      events: "События",
      people: "Люди",
      installs: "Установки",
    },
    breakdown: {
      title: "Свойства",
      property: "Свойство",
      table: "{event} по свойству {property}",
      value: "Значение",
      events: "События",
      people: "Люди",
      installs: "Установки",
      share: "Доля",
      other: "Остальные",
      missing: "Нет свойства",
      noProperties: "У этого события нет свойств за период.",
      empty: "За период нет событий с этим свойством.",
      more: "Показать больше",
    },
  },
};
