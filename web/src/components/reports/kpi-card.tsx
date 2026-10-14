// TODO(T-038): заглушка KpiCard.
export type KpiCardProps = {
  label: string;
  definition: string;
  value: number | null;
  format: "number" | "duration" | "percent";
  deltaPct?: number | null;
  secondary?: string;
  sparkline?: Array<{ ts: string; value: number | null }>;
  selected?: boolean;
  onSelect?: () => void;
  loading?: boolean;
};

export function deltaLabel(_value: number | null): string | null {
  return null;
}

export function KpiCard(_props: KpiCardProps) {
  return null;
}
