import type { metrics as en } from "../en/metrics";

export const metrics: typeof en = {
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
};
