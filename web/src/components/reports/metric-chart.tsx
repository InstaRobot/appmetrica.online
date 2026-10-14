// TODO(T-038): заглушка MetricChart.
import type { TimeSeries } from "@/lib/api/gen/model";
import type { Granularity } from "@/lib/period";
import type { ChartMetric } from "@/lib/url-state";

type MetricChartProps = {
  series: TimeSeries;
  granularity: Granularity;
  metric: ChartMetric;
  timezone: string;
  height?: number;
};

export function MetricChart(_props: MetricChartProps) {
  return null;
}
