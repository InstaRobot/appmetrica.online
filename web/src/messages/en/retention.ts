import { pluralForms } from "@/lib/i18n/format-message";

// Retention S-06: когорты по дням или неделям, средняя кривая удержания и матрица когорт.
export const retention = {
  title: "Retention",
  cohorts: "Cohorts",
  cohortOptions: { day: "By day", week: "By week" },
  empty: {
    title: "No new users in the selected cohorts",
    description: "Retention is calculated for people whose first session fell within the period.",
  },
  average: {
    title: "Average retention curve",
    series: "Average retention",
    keyPoints: "Key retention points",
    // Описание графика для скринридеров: «… по дням: D1 40 %, D7 5,7 %».
    description: "Average retention curve {by}: {points}",
    by: { day: "by day", week: "by week" },
    column: { day: "Day", week: "Week" },
    noData: "no data",
  },
  heatmap: {
    caption: {
      day: "Retention by day: share of the cohort that returned on day N",
      week: "Retention by week: share of the cohort that returned in week N",
    },
    region: "Cohort matrix, scrolls horizontally",
    cohort: "Cohort",
    size: "Size",
    average: "Average",
    noData: "no data",
    period: { day: "day", week: "week" },
    // Подсказка ячейки: когорта, её размер, период и вернувшиеся в абсолютах.
    cellTitle: "Cohort {cohort} ({size}) · {period} {n}: {returned} ({share})",
    people: pluralForms({ one: "{count} person", other: "{count} people" }),
    installs: pluralForms({ one: "{count} install", other: "{count} installs" }),
  },
};
