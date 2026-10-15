import { pluralForms } from "@/lib/i18n/format-message";
import type { retention as en } from "../en/retention";

// «чел.» и «устр.» — сокращения, по числу не склоняются.
export const retention: typeof en = {
  title: "Retention",
  cohorts: "Когорты",
  cohortOptions: { day: "по дням", week: "по неделям" },
  empty: {
    title: "Нет новых пользователей в выбранных когортах",
    description: "Retention считается по людям, чья первая сессия пришлась на период.",
  },
  average: {
    title: "Средняя кривая удержания",
    series: "Среднее удержание",
    keyPoints: "Ключевые точки удержания",
    description: "Средняя кривая удержания {by}: {points}",
    by: { day: "по дням", week: "по неделям" },
    column: { day: "День", week: "Неделя" },
    noData: "нет данных",
  },
  heatmap: {
    caption: {
      day: "Retention по дням: доля когорты, вернувшейся в день N",
      week: "Retention по неделям: доля когорты, вернувшейся в неделю N",
    },
    region: "Матрица когорт, прокручивается по горизонтали",
    cohort: "Когорта",
    size: "Размер",
    average: "Среднее",
    noData: "нет данных",
    period: { day: "день", week: "неделя" },
    cellTitle: "Когорта {cohort} ({size}) · {period} {n}: {returned} ({share})",
    people: pluralForms({
      one: "{count} чел.",
      few: "{count} чел.",
      many: "{count} чел.",
      other: "{count} чел.",
    }),
    installs: pluralForms({
      one: "{count} устр.",
      few: "{count} устр.",
      many: "{count} устр.",
      other: "{count} устр.",
    }),
  },
};
