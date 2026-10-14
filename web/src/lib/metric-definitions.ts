// Заглушка для красного теста T-036; реализация — в следующем коммите.
type MetricDefinition = { label: string; definition: string };
export const METRIC_DEFINITIONS: Record<
  | "active"
  | "new"
  | "sessions"
  | "avg_duration_ms"
  | "median_duration_ms"
  | "retention_d1"
  | "active_now",
  MetricDefinition
> = {
  active: { label: "", definition: "" },
  new: { label: "", definition: "" },
  sessions: { label: "", definition: "" },
  avg_duration_ms: { label: "", definition: "" },
  median_duration_ms: { label: "", definition: "" },
  retention_d1: { label: "", definition: "" },
  active_now: { label: "", definition: "" },
};
