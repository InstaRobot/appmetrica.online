import type { RetentionMatrix, Unit } from "@/lib/api/gen/model";

type RetentionHeatmapProps = {
  matrix: RetentionMatrix;
  granularity: "day" | "week";
  unit?: Unit;
};

export function RetentionHeatmap(_props: RetentionHeatmapProps) {
  return null;
}
