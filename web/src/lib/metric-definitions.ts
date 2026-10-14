// Названия метрик и тексты tooltip `Info` у KPI (14-ux-ui-spec.md §14.2, §15.4). Определения повторяют
// 02-tech-spec.md (FR-013–FR-015) и 05-data-model.md: меняются только вместе с ними.
type MetricDefinition = { label: string; definition: string };

export const METRIC_DEFINITIONS = {
  active: {
    label: "Активные",
    definition:
      "Люди, у которых за период была хотя бы одна сессия. Человек — user_id после входа или установка без входа.",
  },
  new: {
    label: "Новые",
    definition: "Люди, чьё первое событие в приложении пришлось на выбранный период.",
  },
  sessions: {
    label: "Сессии",
    definition: "Число сессий: каждая сессия начинается событием session_start.",
  },
  avg_duration_ms: {
    label: "Ср. длительность",
    definition: "Средняя длительность сессий, завершённых событием session_end с duration_ms.",
  },
  median_duration_ms: {
    label: "Медиана",
    definition: "Медиана длительности сессий, завершённых событием session_end с duration_ms.",
  },
  retention_d1: {
    label: "D1",
    definition: "Доля когорты новых, вернувшихся ровно на следующий день после первого события.",
  },
  retention_d7: {
    label: "D7",
    definition: "Доля когорты новых, вернувшихся ровно на седьмой день после первого события.",
  },
  active_now: {
    label: "Активны сейчас",
    definition: "Люди с session_start за последние 30 минут, у которых сессия ещё не завершена.",
  },
} as const satisfies Record<string, MetricDefinition>;

export type MetricKey = keyof typeof METRIC_DEFINITIONS;
