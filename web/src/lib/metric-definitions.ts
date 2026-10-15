import { useMessages } from "@/lib/i18n/client";
import type { Messages } from "@/messages";

// Названия метрик и тексты tooltip `Info` у KPI (14-ux-ui-spec.md §14.2, §15.4) — словарь metrics
// (messages/<язык>/metrics.ts): определения повторяют 02-tech-spec.md и 05-data-model.md.
export type MetricDefinition = { label: string; definition: string };

export type MetricDefinitions = Messages["metrics"];

export type MetricKey = keyof MetricDefinitions;

// Определения метрик на языке интерфейса — для клиентских компонентов под MessagesProvider.
export function useMetricDefinitions(): Record<MetricKey, MetricDefinition> {
  return useMessages().metrics;
}
