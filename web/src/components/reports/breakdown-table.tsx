// TODO(T-038): заглушка BreakdownTable.
import type { Breakdown } from "@/lib/api/gen/model";
import type { BreakdownDim, BreakdownMetricKey } from "@/lib/url-state";

type BreakdownTableProps = {
  dimension: BreakdownDim;
  onDimensionChange: (d: BreakdownDim) => void;
  metric: BreakdownMetricKey;
  onMetricChange: (m: BreakdownMetricKey) => void;
  data: Breakdown | undefined;
  onRowClick?: (value: string) => void;
  loading?: boolean;
  error?: React.ReactNode;
};

export function BreakdownTable(_props: BreakdownTableProps) {
  return null;
}
