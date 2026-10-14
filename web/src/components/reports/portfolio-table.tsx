import type { PortfolioRow } from "@/lib/api/gen/model";

export type PortfolioSortKey =
  | "name"
  | "active_today"
  | "active_date"
  | "active_delta_pct"
  | "new_date"
  | "sessions_date"
  | "retention_d1"
  | "retention_d7";
export type PortfolioSort = { key: PortfolioSortKey; desc: boolean };

type PortfolioTableProps = {
  rows: PortfolioRow[];
  date: string;
  sort: PortfolioSort;
  onSortChange: (sort: PortfolioSort) => void;
  onOpenApp?: (appId: string) => void;
};

// TODO(T-037)
export function PortfolioTable(_props: PortfolioTableProps) {
  return null;
}
