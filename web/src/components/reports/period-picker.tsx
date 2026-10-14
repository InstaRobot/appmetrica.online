// TODO(T-038): заглушка PeriodPicker.
import type { PeriodValue } from "@/lib/period";

type PeriodPickerProps = {
  value: PeriodValue;
  onChange: (value: PeriodValue) => void;
  timezone: string;
  maxRangeDays?: number;
  error?: string | undefined;
};

export function PeriodPicker(_props: PeriodPickerProps) {
  return null;
}
