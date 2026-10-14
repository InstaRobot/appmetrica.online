// TODO(T-038): заглушка AudienceKpis.
import type { AudienceSummary, TimeSeriesPoint } from "@/lib/api/gen/model";
import type { ChartMetric } from "@/lib/url-state";

type AudienceKpisProps = {
  summary: AudienceSummary | undefined;
  sparklines?: Partial<Record<ChartMetric, TimeSeriesPoint[]>>;
  selected: ChartMetric;
  onSelect: (metric: ChartMetric) => void;
  loading?: boolean;
  error?: React.ReactNode;
};

export function AudienceKpis(_props: AudienceKpisProps) {
  return null;
}
