// Названия метрик и тексты tooltip `Info` у KPI (14-ux-ui-spec.md §14.2, §15.4). Определения повторяют
// 02-tech-spec.md (FR-013–FR-015) и 05-data-model.md: меняются только вместе с ними.
export const metrics = {
  active: {
    label: "Active users",
    definition:
      "People who had at least one session in the period. A person is a user_id after sign-in or an install without sign-in.",
  },
  new: {
    label: "New users",
    definition: "People whose first event in the app fell within the selected period.",
  },
  sessions: {
    label: "Sessions",
    definition: "Number of sessions: each session starts with a session_start event.",
  },
  avg_duration_ms: {
    label: "Avg. duration",
    definition: "Average duration of sessions ended by a session_end event with duration_ms.",
  },
  median_duration_ms: {
    label: "Median",
    definition: "Median duration of sessions ended by a session_end event with duration_ms.",
  },
  retention_d1: {
    label: "D1",
    definition:
      "Share of the new-user cohort that came back exactly on the day after their first event.",
  },
  retention_d7: {
    label: "D7",
    definition:
      "Share of the new-user cohort that came back exactly on the seventh day after their first event.",
  },
  active_now: {
    label: "Active now",
    definition:
      "People with a session_start in the last 30 minutes whose session hasn't ended yet.",
  },
};
