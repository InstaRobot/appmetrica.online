"use client";

import { useId } from "react";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type { Breakdown } from "@/lib/api/gen/model";
import { formatInt, formatPercent } from "@/lib/format";
import { METRIC_DEFINITIONS } from "@/lib/metric-definitions";
import {
  type BreakdownDim,
  type BreakdownMetricKey,
  FILTER_DIMENSIONS,
  type FilterDimension,
} from "@/lib/url-state";

type BreakdownTableProps = {
  dimension: BreakdownDim;
  onDimensionChange: (d: BreakdownDim) => void;
  metric: BreakdownMetricKey;
  onMetricChange: (m: BreakdownMetricKey) => void;
  data: Breakdown | undefined;
  /** Добавляет фильтр по значению строки (только для platform, app_version, os_name, country). */
  onRowClick?: (value: string) => void;
  loading?: boolean;
  /** ErrorState вместо таблицы. */
  error?: React.ReactNode;
};

// Порядок вкладок — 14-ux-ui-spec.md S-05.
export const BREAKDOWN_LABELS: Record<BreakdownDim, string> = {
  app_version: "Версия приложения",
  os_name: "ОС",
  os_version: "Версия ОС",
  device_model: "Устройство",
  device_type: "Тип устройства",
  country: "Страна",
  city: "Город",
  locale: "Язык",
  platform: "Платформа",
};

const DIMENSIONS = Object.keys(BREAKDOWN_LABELS) as BreakdownDim[];
const METRICS: BreakdownMetricKey[] = ["active", "new", "sessions"];

const isFilterable = (d: BreakdownDim): d is FilterDimension =>
  (FILTER_DIMENSIONS as readonly string[]).includes(d);

// Разбивка S-05 (14-ux-ui-spec.md §16.8): значение, метрика и доля от значения без разбивки.
export function BreakdownTable({
  dimension,
  onDimensionChange,
  metric,
  onMetricChange,
  data,
  onRowClick,
  loading = false,
  error,
}: BreakdownTableProps) {
  const metricLabelId = useId();
  const label = BREAKDOWN_LABELS[dimension];
  const clickable = onRowClick !== undefined && isFilterable(dimension);
  const rows = data?.rows ?? [];

  let body: React.ReactNode = null;
  if (error) {
    body = error;
  } else if (loading && !data) {
    body = <TableSkeleton rows={6} />;
  } else if (data && rows.length === 0) {
    body = (
      <p className="py-6 text-center text-body-md text-fg-muted">Нет данных за выбранный период</p>
    );
  } else if (data) {
    body = (
      <Table aria-label={`Разбивка: ${label}`}>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead scope="col" className="px-3">
              Значение
            </TableHead>
            <TableHead scope="col" className="px-3 text-right">
              {METRIC_DEFINITIONS[metric].label}
            </TableHead>
            <TableHead scope="col" className="w-2/5 px-3">
              Доля
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.value}>
              <TableCell className="px-3">
                {row.value === "" ? (
                  <span className="text-fg-muted">Не определено</span>
                ) : clickable ? (
                  <button
                    type="button"
                    aria-label={`Фильтр: ${label} ${row.value}`}
                    onClick={() => onRowClick(row.value)}
                    className="rounded-sm text-fg-link hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                  >
                    {row.value}
                  </button>
                ) : (
                  row.value
                )}
              </TableCell>
              <TableCell className="px-3 text-right tabular-nums">{formatInt(row.count)}</TableCell>
              <TableCell className="px-3">
                <span className="flex items-center gap-2">
                  <span
                    aria-hidden="true"
                    className="h-2 flex-1 overflow-hidden rounded-full bg-subtle"
                  >
                    <span
                      className="block h-full rounded-full bg-chart-1"
                      style={{ width: `${Math.min(row.share, 1) * 100}%` }}
                    />
                  </span>
                  <span className="w-14 text-right tabular-nums">{formatPercent(row.share)}</span>
                </span>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          spacing={0}
          aria-label="Измерение"
          value={dimension}
          onValueChange={(next) => {
            const d = DIMENSIONS.find((x) => x === next);
            if (d) onDimensionChange(d);
          }}
          className="flex-wrap"
        >
          {DIMENSIONS.map((d) => (
            <ToggleGroupItem key={d} value={d} className="px-3">
              {BREAKDOWN_LABELS[d]}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <div className="flex items-center gap-2">
          <span id={metricLabelId} className="text-label-md text-fg-muted">
            Метрика
          </span>
          <Select
            value={metric}
            onValueChange={(next) => {
              const m = METRICS.find((x) => x === next);
              if (m) onMetricChange(m);
            }}
          >
            <SelectTrigger size="sm" aria-labelledby={metricLabelId}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {METRICS.map((m) => (
                <SelectItem key={m} value={m}>
                  {METRIC_DEFINITIONS[m].label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      {body}
      {metric === "active" && rows.length > 0 && !error ? (
        <p className="text-caption text-fg-muted">
          Сумма долей может превышать 100 %: человек мог пользоваться несколькими значениями за
          период.
        </p>
      ) : null}
    </div>
  );
}
