// TODO(T-038): заглушка GranularitySelect.
import type { Granularity } from "@/lib/period";

type GranularitySelectProps = {
  value: Granularity;
  allowed: Granularity[];
  onChange: (value: Granularity) => void;
};

export function GranularitySelect(_props: GranularitySelectProps) {
  return null;
}
