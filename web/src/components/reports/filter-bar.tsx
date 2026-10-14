// TODO(T-038): заглушка FilterBar.
import type { FilterDimension, Filters } from "@/lib/url-state";

type FilterBarProps = {
  value: Filters;
  onChange: (value: Filters) => void;
  options: Partial<Record<FilterDimension, Array<{ value: string; count: number }>>>;
  onOpenChange?: (open: boolean) => void;
  loading?: boolean;
};

export function FilterBar(_props: FilterBarProps) {
  return null;
}
